package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Schema represents a JSON Schema specification for OpenAI-compatible Structured Outputs.
type Schema struct {
	Type        []string   // e.g. ["string"] or ["string", "null"]
	Description string     // semantic instruction to guide the model on this field
	Enum        []any      // allowed values (may include nil if nullable)
	Properties  []Property // ordered properties for objects
	Items       *Schema    // schema for array items
}

// Property pairs a field name with its Schema definition, preserving definition order.
type Property struct {
	Name   string
	Schema *Schema
}

// Prop constructs an ordered property definition.
func Prop(name string, schema *Schema) Property {
	return Property{Name: name, Schema: schema}
}

// Obj constructs an object schema with ordered properties.
func Obj(desc string, props ...Property) *Schema {
	return &Schema{
		Type:        []string{"object"},
		Description: desc,
		Properties:  props,
	}
}

// Str constructs a string schema.
func Str(desc string) *Schema {
	return &Schema{
		Type:        []string{"string"},
		Description: desc,
	}
}

// Num constructs a number schema.
func Num(desc string) *Schema {
	return &Schema{
		Type:        []string{"number"},
		Description: desc,
	}
}

// Bool constructs a boolean schema.
func Bool(desc string) *Schema {
	return &Schema{
		Type:        []string{"boolean"},
		Description: desc,
	}
}

// Enum constructs a string schema constrained to specific allowed values.
func Enum(desc string, values ...string) *Schema {
	enums := make([]any, len(values))
	for i, v := range values {
		enums[i] = v
	}
	return &Schema{
		Type:        []string{"string"},
		Description: desc,
		Enum:        enums,
	}
}

// Arr constructs an array schema with a specified item schema.
func Arr(desc string, items *Schema) *Schema {
	return &Schema{
		Type:        []string{"array"},
		Description: desc,
		Items:       items,
	}
}

// Nullable marks an existing schema as nullable (e.g. ["string", "null"]).
func Nullable(s *Schema) *Schema {
	if s == nil {
		return &Schema{Type: []string{"null"}}
	}
	cp := *s
	hasNull := false
	for _, t := range cp.Type {
		if t == "null" {
			hasNull = true
			break
		}
	}
	if !hasNull {
		cp.Type = append(append([]string{}, cp.Type...), "null")
	}
	if len(cp.Enum) > 0 {
		hasNilEnum := false
		for _, e := range cp.Enum {
			if e == nil {
				hasNilEnum = true
				break
			}
		}
		if !hasNilEnum {
			cp.Enum = append(append([]any{}, cp.Enum...), nil)
		}
	}
	return &cp
}

// MarshalJSON emits an ordered JSON representation with strict mode invariants:
// - properties keys are emitted in the exact order of s.Properties
// - required is emitted with all property names in that exact same order
// - additionalProperties is emitted as false on all objects
func (s *Schema) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}

	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true

	writeKey := func(key string) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		k, _ := json.Marshal(key)
		buf.Write(k)
		buf.WriteByte(':')
	}

	// 1. type
	if len(s.Type) == 1 {
		writeKey("type")
		t, _ := json.Marshal(s.Type[0])
		buf.Write(t)
	} else if len(s.Type) > 1 {
		writeKey("type")
		t, _ := json.Marshal(s.Type)
		buf.Write(t)
	}

	// 2. description
	if s.Description != "" {
		writeKey("description")
		d, _ := json.Marshal(s.Description)
		buf.Write(d)
	}

	// 3. enum
	if len(s.Enum) > 0 {
		writeKey("enum")
		e, err := json.Marshal(s.Enum)
		if err != nil {
			return nil, err
		}
		buf.Write(e)
	}

	// 4. properties, required, additionalProperties (for objects)
	isObject := false
	for _, t := range s.Type {
		if t == "object" {
			isObject = true
			break
		}
	}

	if isObject {
		writeKey("properties")
		buf.WriteByte('{')
		for i, prop := range s.Properties {
			if i > 0 {
				buf.WriteByte(',')
			}
			pk, _ := json.Marshal(prop.Name)
			buf.Write(pk)
			buf.WriteByte(':')
			pv, err := json.Marshal(prop.Schema)
			if err != nil {
				return nil, err
			}
			buf.Write(pv)
		}
		buf.WriteByte('}')

		// Strict mode requires all properties in "required" in the exact same sequence
		writeKey("required")
		buf.WriteByte('[')
		for i, prop := range s.Properties {
			if i > 0 {
				buf.WriteByte(',')
			}
			pk, _ := json.Marshal(prop.Name)
			buf.Write(pk)
		}
		buf.WriteByte(']')

		writeKey("additionalProperties")
		buf.WriteString("false")
	}

	// 5. items (for arrays)
	isArray := false
	for _, t := range s.Type {
		if t == "array" {
			isArray = true
			break
		}
	}
	if isArray && s.Items != nil {
		writeKey("items")
		iv, err := json.Marshal(s.Items)
		if err != nil {
			return nil, err
		}
		buf.Write(iv)
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// ValidateStrict verifies that a schema conforms to the strict Structured Outputs allow-list:
// - Objects must have at least one property and additionalProperties false
// - Arrays must define an items schema
// - Enums must be non-empty
// - Max nesting depth <= 5
func ValidateStrict(s *Schema) error {
	return validateStrictRec(s, 0)
}

func validateStrictRec(s *Schema, depth int) error {
	if s == nil {
		return errors.New("schema is nil")
	}
	if depth > 5 {
		return fmt.Errorf("schema nesting depth %d exceeds maximum allowed (5)", depth)
	}

	if len(s.Type) == 0 {
		return errors.New("schema type must not be empty")
	}

	for _, t := range s.Type {
		switch t {
		case "string", "number", "integer", "boolean", "object", "array", "null":
		default:
			return fmt.Errorf("unsupported type %q", t)
		}
	}

	isObject := false
	isArray := false
	for _, t := range s.Type {
		if t == "object" {
			isObject = true
		}
		if t == "array" {
			isArray = true
		}
	}

	if isObject {
		if len(s.Properties) == 0 {
			return errors.New("object schema must define at least one property")
		}
		seen := make(map[string]bool)
		for _, prop := range s.Properties {
			if prop.Name == "" {
				return errors.New("property name cannot be empty")
			}
			if seen[prop.Name] {
				return fmt.Errorf("duplicate property %q in object schema", prop.Name)
			}
			seen[prop.Name] = true
			if err := validateStrictRec(prop.Schema, depth+1); err != nil {
				return fmt.Errorf("property %q: %w", prop.Name, err)
			}
		}
	}

	if isArray {
		if s.Items == nil {
			return errors.New("array schema must define items")
		}
		if err := validateStrictRec(s.Items, depth+1); err != nil {
			return fmt.Errorf("array items: %w", err)
		}
	}

	return nil
}

// PromptText renders a compact human-readable Markdown/text representation of the schema and descriptions.
func (s *Schema) PromptText() string {
	if s == nil {
		return ""
	}
	var sb strings.Builder
	s.renderPromptText(&sb, 0)
	return sb.String()
}

func (s *Schema) renderPromptText(sb *strings.Builder, indent int) {
	pad := strings.Repeat("  ", indent)

	isObject := false
	isArray := false
	for _, t := range s.Type {
		if t == "object" {
			isObject = true
		}
		if t == "array" {
			isArray = true
		}
	}

	if isObject {
		sb.WriteString("{\n")
		for i, prop := range s.Properties {
			if i > 0 {
				sb.WriteString(",\n")
			}
			if prop.Schema.Description != "" {
				sb.WriteString(fmt.Sprintf("%s  // %s\n", pad, prop.Schema.Description))
			}
			sb.WriteString(fmt.Sprintf("%s  %q: ", pad, prop.Name))
			prop.Schema.renderPromptText(sb, indent+1)
		}
		sb.WriteString("\n" + pad + "}")
		return
	}

	if isArray && s.Items != nil {
		sb.WriteString("[\n")
		sb.WriteString(pad + "  ")
		s.Items.renderPromptText(sb, indent+1)
		sb.WriteString("\n" + pad + "]")
		return
	}

	if len(s.Enum) > 0 {
		var vals []string
		for _, e := range s.Enum {
			if e == nil {
				vals = append(vals, "null")
			} else {
				vals = append(vals, fmt.Sprintf("%q", fmt.Sprint(e)))
			}
		}
		sb.WriteString(strings.Join(vals, " | "))
		return
	}

	sb.WriteString(strings.Join(s.Type, "|"))
}
