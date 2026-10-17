package sim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPServer provides a lightweight HTTP interface for human controller playthroughs.
type HTTPServer struct {
	world     *Simulation
	humanCtrl *HumanController
	addr      string
	listener  net.Listener
	server    *http.Server

	mu             sync.Mutex
	currentTurn    *Turn
	lastResolution string
	lastRulingTick int
	lastAction     string
	gameOver       bool
	err            error
	turnWaiters    []chan struct{}
}

// NewHTTPServer constructs an HTTP interactive play server.
func NewHTTPServer(world *Simulation, humanCtrl *HumanController, addr string) *HTTPServer {
	if addr == "" {
		addr = ":8080"
	}
	return &HTTPServer{
		world:     world,
		humanCtrl: humanCtrl,
		addr:      addr,
	}
}

// Addr returns the network address the server is listening on.
func (s *HTTPServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

// Start begins listening and serving HTTP requests, and pumps turns from the HumanController.
func (s *HTTPServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/turn", s.handleGetTurn)
	mux.HandleFunc("/turn/text", s.handleGetTurnText)
	mux.HandleFunc("/action", s.handlePostAction)
	mux.HandleFunc("/reply", s.handlePostAction)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/quit", s.handleQuit)

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("http server failed to listen on %s: %w", s.addr, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second,
	}
	s.mu.Unlock()

	// Goroutine: pump turns from human controller into HTTPServer
	go func() {
		for {
			select {
			case turn, ok := <-s.humanCtrl.Turns():
				if !ok {
					s.mu.Lock()
					s.gameOver = true
					s.notifyWaitersLocked()
					s.mu.Unlock()
					return
				}
				s.mu.Lock()
				t := turn
				s.currentTurn = &t
				s.notifyWaitersLocked()
				s.mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Goroutine: graceful shutdown on context cancellation
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
	}()

	// Goroutine: serve HTTP requests
	go func() {
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	return nil
}

// OnTickReport updates server state with Arbiter rulings and actor actions.
func (s *HTTPServer) OnTickReport(report TickReport) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if report.SceneRuling != nil {
		s.lastResolution = report.SceneRuling.Resolution
		s.lastRulingTick = report.Tick
	}
	if report.ActorID == "zack" {
		s.lastAction = report.Reaction.Action.Action
	}
}

// SetGameOver marks the simulation as completed.
func (s *HTTPServer) SetGameOver(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.gameOver = true
	s.err = err
	s.currentTurn = nil
	s.notifyWaitersLocked()
}

func (s *HTTPServer) notifyWaitersLocked() {
	for _, ch := range s.turnWaiters {
		close(ch)
	}
	s.turnWaiters = nil
}

func (s *HTTPServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, `==================================================
   OMNIA HTTP INTERACTIVE PLAY SERVER
==================================================
Player: Zack | Scenario: Accidental Arrangement

ENDPOINTS:
  GET  /turn         Await / inspect current turn (JSON by default; ?wait=false to not block)
  GET  /turn/text    Current turn formatted as plain text
  POST /action       Submit player action/speech (accepts JSON {"action":"..."} or raw text)
  GET  /status       Current simulation status, tick, clock
  GET  /events       Simulation event log
  POST /quit         Terminate simulation cleanly

CURL EXAMPLES:
  # Check / wait for your turn:
  curl -s http://localhost:8080/turn | jq .

  # Or view plain text turn prompt:
  curl -s http://localhost:8080/turn/text

  # Submit what you say or do:
  curl -s -X POST http://localhost:8080/action -d '{"action": "Notes are good. Shows dedication."}'
  # or raw string:
  curl -s -X POST http://localhost:8080/action -d "I sit on the couch and smile dryly."

  # Check event log:
  curl -s http://localhost:8080/events | jq .
==================================================
`)
}

func (s *HTTPServer) handleGetTurnText(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	q.Set("format", "text")
	r.URL.RawQuery = q.Encode()
	s.handleGetTurn(w, r)
}

func (s *HTTPServer) handleGetTurn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wait := r.URL.Query().Get("wait") != "false"
	timeout := 60 * time.Second
	if timeoutStr := r.URL.Query().Get("timeout"); timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil && d > 0 {
			timeout = d
		}
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		if strings.Contains(r.Header.Get("Accept"), "text/plain") {
			format = "text"
		} else {
			format = "json"
		}
	}

	s.mu.Lock()
	if s.currentTurn == nil && !s.gameOver && wait {
		waitCh := make(chan struct{})
		s.turnWaiters = append(s.turnWaiters, waitCh)
		s.mu.Unlock()

		select {
		case <-waitCh:
			s.mu.Lock()
		case <-time.After(timeout):
			s.mu.Lock()
			for i, ch := range s.turnWaiters {
				if ch == waitCh {
					s.turnWaiters = append(s.turnWaiters[:i], s.turnWaiters[i+1:]...)
					break
				}
			}
		case <-r.Context().Done():
			s.mu.Lock()
			for i, ch := range s.turnWaiters {
				if ch == waitCh {
					s.turnWaiters = append(s.turnWaiters[:i], s.turnWaiters[i+1:]...)
					break
				}
			}
			s.mu.Unlock()
			return
		}
	}

	turn := s.currentTurn
	gameOver := s.gameOver
	lastRes := s.lastResolution
	lastAct := s.lastAction
	s.mu.Unlock()

	if gameOver && turn == nil {
		if format == "text" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintf(w, "Simulation ended.\nLast Resolution: %s\n", lastRes)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"waiting":         false,
			"game_over":       true,
			"last_resolution": lastRes,
			"message":         "Simulation has ended",
		})
		return
	}

	if turn == nil {
		if format == "text" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "No turn currently pending (simulation is calculating or idle).")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"waiting":         false,
			"game_over":       false,
			"last_resolution": lastRes,
			"message":         "No turn currently pending",
		})
		return
	}

	displayText := s.formatTurnDisplay(turn, lastRes, lastAct)
	if format == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, displayText)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"waiting":         true,
		"game_over":       false,
		"tick":            turn.Tick,
		"timestamp":       turn.Timestamp,
		"actor":           turn.ActorName,
		"actor_id":        turn.ActorID,
		"stimulus":        turn.Stimulus,
		"thoughts":        turn.Thoughts,
		"last_resolution": lastRes,
		"display":         displayText,
	})
}

func (s *HTTPServer) formatTurnDisplay(turn *Turn, lastResolution, lastAction string) string {
	var sb strings.Builder
	sb.WriteString("==================================================\n")
	sb.WriteString(fmt.Sprintf(">>> YOUR TURN: %s [%s] (Tick %d) <<<\n", turn.ActorName, turn.Timestamp, turn.Tick))
	sb.WriteString("==================================================\n")

	if lastResolution != "" {
		sb.WriteString("\n--- Previous Scene Resolution ---\n")
		sb.WriteString(lastResolution)
		sb.WriteString("\n")
	}

	if turn.Thoughts != "" {
		sb.WriteString("\n--- Inner Thoughts (Zack) ---\n")
		sb.WriteString(turn.Thoughts)
		sb.WriteString("\n")
	}

	sb.WriteString("\n--- Current Situation & Surroundings ---\n")
	sb.WriteString(turn.Stimulus)
	sb.WriteString("\n")

	sb.WriteString("\nWhat do you say or do?\n")
	sb.WriteString("> POST /action -d '{\"action\": \"...\"}' OR curl -d 'your action' http://localhost:8080/action\n")
	sb.WriteString("==================================================\n")
	return sb.String()
}

func (s *HTTPServer) handlePostAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var actionText string
	var req struct {
		Action string `json:"action"`
		Input  string `json:"input"`
	}
	if err := json.Unmarshal(body, &req); err == nil && (req.Action != "" || req.Input != "") {
		if req.Action != "" {
			actionText = req.Action
		} else {
			actionText = req.Input
		}
	} else {
		actionText = strings.TrimSpace(string(body))
	}

	if actionText == "" {
		http.Error(w, `{"error":"action cannot be empty"}`, http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.gameOver {
		s.mu.Unlock()
		http.Error(w, `{"error":"simulation has ended"}`, http.StatusGone)
		return
	}
	if s.currentTurn == nil {
		s.mu.Unlock()
		http.Error(w, `{"error":"no pending turn waiting for player action"}`, http.StatusConflict)
		return
	}

	turnTick := s.currentTurn.Tick
	s.currentTurn = nil
	s.mu.Unlock()

	if actionText == "quit" || actionText == "exit" {
		s.humanCtrl.Quit()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "quit",
			"message": "Player quit the simulation",
		})
		return
	}

	go s.humanCtrl.Reply(actionText)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"action":  actionText,
		"tick":    turnTick,
		"message": "Action accepted. Deliberating next tick...",
	})
}

func (s *HTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"running":            !s.gameOver,
		"game_over":          s.gameOver,
		"waiting_for_player": s.currentTurn != nil,
		"tick":               s.world.Clock.TickCount,
		"timestamp":          s.world.Clock.Timestamp(),
		"last_resolution":    s.lastResolution,
		"last_action":        s.lastAction,
	})
}

func (s *HTTPServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	events := s.world.GetEventLog()
	format := r.URL.Query().Get("format")
	if format == "text" || strings.Contains(r.Header.Get("Accept"), "text/plain") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for i, ev := range events {
			fmt.Fprintf(w, "[%d] Tick %d | Kind: %-7s | Source: %-15s | Loc: %s\n    %s\n",
				i+1, ev.Tick, ev.Kind, ev.SourceName, ev.LocationID, ev.Description)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tick":   s.world.Clock.TickCount,
		"count":  len(events),
		"events": events,
	})
}

func (s *HTTPServer) handleQuit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.humanCtrl.Quit()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "quit",
		"message": "Simulation termination requested",
	})
}
