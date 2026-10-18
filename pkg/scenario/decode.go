package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"omnia.adityagupta.dev/pkg/scenario/spec"
)

const (
	// DefaultMaxScenarioSize limits scenario documents to 256 MB by default.
	DefaultMaxScenarioSize = 256 * 1024 * 1024
	// SupportedFormatName is the canonical specification identifier.
	SupportedFormatName = "omnia.scenario"
	// SupportedMajorVersion is the engine's current major spec version.
	SupportedMajorVersion = 1
	// SupportedMinorVersion is the engine's current minor spec version.
	SupportedMinorVersion = 0
)

// DecodeOptions configures document reading and decoding limits.
type DecodeOptions struct {
	MaxBytes int64
}

// Decode reads, size-gates, checks for duplicate keys, version-gates, and strictly decodes an Omnia scenario document.
func Decode(r io.Reader, opts ...DecodeOptions) (*spec.Document, error) {
	maxBytes := int64(DefaultMaxScenarioSize)
	if len(opts) > 0 && opts[0].MaxBytes > 0 {
		maxBytes = opts[0].MaxBytes
	}

	lr := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("read error: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("scenario file exceeds maximum allowed size of %d bytes", maxBytes)
	}

	// 1. Duplicate-key scan
	if err := scanDuplicateKeys(data); err != nil {
		return nil, err
	}

	// 2. Lenient version gate
	var versionGate struct {
		Format struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"format"`
	}
	if err := json.Unmarshal(data, &versionGate); err != nil {
		return nil, fmt.Errorf("invalid json format: %w", err)
	}

	if versionGate.Format.Name != SupportedFormatName {
		return nil, fmt.Errorf("unsupported format name %q (expected %q)", versionGate.Format.Name, SupportedFormatName)
	}

	parts := strings.Split(versionGate.Format.Version, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid format version %q (expected major.minor, e.g. 1.0)", versionGate.Format.Version)
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("invalid format version %q: major and minor must be integers", versionGate.Format.Version)
	}

	if major != SupportedMajorVersion {
		return nil, fmt.Errorf("unsupported format major version %d (engine supports %d.x)", major, SupportedMajorVersion)
	}
	if minor > SupportedMinorVersion {
		return nil, fmt.Errorf("scenario requires format %s; engine supports <= %d.%d",
			versionGate.Format.Version, SupportedMajorVersion, SupportedMinorVersion)
	}

	// 3. Strict typed decode
	var doc spec.Document
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("strict schema decode error: %w", err)
	}

	return &doc, nil
}

type containerKind int

const (
	containerObject containerKind = iota
	containerArray
)

type containerState struct {
	kind      containerKind
	expectKey bool
	keys      map[string]bool
}

func scanDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var stack []containerState

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("syntax error: %w", err)
		}

		// Delimiters take precedence
		if t, isDelim := tok.(json.Delim); isDelim {
			switch t {
			case '{':
				if len(stack) > 0 && stack[len(stack)-1].kind == containerObject && !stack[len(stack)-1].expectKey {
					stack[len(stack)-1].expectKey = true
				}
				stack = append(stack, containerState{
					kind:      containerObject,
					expectKey: true,
					keys:      make(map[string]bool),
				})
				continue
			case '[':
				if len(stack) > 0 && stack[len(stack)-1].kind == containerObject && !stack[len(stack)-1].expectKey {
					stack[len(stack)-1].expectKey = true
				}
				stack = append(stack, containerState{
					kind:      containerArray,
					expectKey: false,
				})
				continue
			case '}', ']':
				if len(stack) == 0 {
					return fmt.Errorf("unexpected closing delimiter %q", t)
				}
				stack = stack[:len(stack)-1]
				if len(stack) > 0 && stack[len(stack)-1].kind == containerObject {
					stack[len(stack)-1].expectKey = true
				}
				continue
			}
		}

		if len(stack) > 0 && stack[len(stack)-1].kind == containerObject {
			top := &stack[len(stack)-1]
			if top.expectKey {
				key, ok := tok.(string)
				if !ok {
					return fmt.Errorf("expected string object key, got %T (%v)", tok, tok)
				}
				if top.keys[key] {
					return fmt.Errorf("duplicate key %q detected in JSON object", key)
				}
				top.keys[key] = true
				top.expectKey = false
				continue
			} else {
				top.expectKey = true
				continue
			}
		}
	}

	return nil
}
