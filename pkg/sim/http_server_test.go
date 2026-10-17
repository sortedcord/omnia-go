package sim

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPServerLifecycle(t *testing.T) {
	world := offlineWorld()
	humanCtrl := NewHumanController(nil, "gemini-3.5-flash-lite")

	server := NewHTTPServer(world, humanCtrl, "127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("failed to start http server: %v", err)
	}

	baseURL := "http://" + server.Addr()

	// 1. Check GET / (index help)
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "OMNIA HTTP INTERACTIVE PLAY SERVER") {
		t.Fatalf("unexpected index response: %s", body)
	}

	// 2. Check GET /turn?wait=false before any turn is queued
	resp, err = http.Get(baseURL + "/turn?wait=false")
	if err != nil {
		t.Fatalf("GET /turn failed: %v", err)
	}
	defer resp.Body.Close()
	var turnResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&turnResp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if turnResp["waiting"] != false {
		t.Fatalf("expected waiting=false, got %v", turnResp["waiting"])
	}

	// 3. Queue a fake turn in humanCtrl
	fakeTurn := Turn{
		ActorID:   "zack",
		ActorName: "Zack",
		Tick:      1,
		Timestamp: "18:35:00 UTC",
		Stimulus:  "Nelly is staring at you in panic.",
		Thoughts:  "What did I get myself into?",
	}
	humanCtrl.turns <- fakeTurn

	// Allow goroutine to pump
	time.Sleep(50 * time.Millisecond)

	// 4. Check GET /turn returns the turn
	resp, err = http.Get(baseURL + "/turn")
	if err != nil {
		t.Fatalf("GET /turn failed: %v", err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&turnResp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if turnResp["waiting"] != true || turnResp["actor"] != "Zack" {
		t.Fatalf("unexpected turn response: %+v", turnResp)
	}

	// 5. Check GET /turn/text returns plain text
	resp, err = http.Get(baseURL + "/turn/text")
	if err != nil {
		t.Fatalf("GET /turn/text failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), ">>> YOUR TURN: Zack") {
		t.Fatalf("unexpected text response: %s", body)
	}

	// 6. Check POST /action
	reqBody, _ := json.Marshal(map[string]string{"action": "I smile and take a seat."})
	resp, err = http.Post(baseURL+"/action", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /action failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify reply was dispatched to humanCtrl.replies
	select {
	case reply := <-humanCtrl.replies:
		if reply != "I smile and take a seat." {
			t.Fatalf("expected reply 'I smile and take a seat.', got %q", reply)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for reply on humanCtrl.replies")
	}

	// 7. Check POST /action without active turn returns 409 Conflict
	resp, err = http.Post(baseURL+"/action", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /action failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", resp.StatusCode)
	}

	// 8. Check GET /status
	resp, err = http.Get(baseURL + "/status")
	if err != nil {
		t.Fatalf("GET /status failed: %v", err)
	}
	defer resp.Body.Close()
	var statusResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Fatalf("status decode failed: %v", err)
	}
	if statusResp["running"] != true {
		t.Fatalf("expected running=true, got %+v", statusResp)
	}

	// 9. Check POST /quit
	resp, err = http.Post(baseURL+"/quit", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /quit failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for quit, got %d", resp.StatusCode)
	}

	select {
	case <-humanCtrl.quitChan:
		// Succeeded!
	case <-time.After(500 * time.Millisecond):
		t.Fatal("humanCtrl was not closed on quit")
	}
}
