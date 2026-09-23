package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SpecInput is a Poly specification input file (functions, webhooks, schemas).
// Title is used only when naming generate output; it is not written back.
type SpecInput struct {
	Title     string           `json:"title,omitempty"`
	Functions []map[string]any `json:"functions"`
	Webhooks  []map[string]any `json:"webhooks"`
	Schemas   []map[string]any `json:"schemas"`
}

func decodeSpec(v any) (SpecInput, error) {
	if v == nil {
		return SpecInput{}, fmt.Errorf("empty specification input")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return SpecInput{}, err
	}
	return Parse(raw)
}

// Parse decodes a specification input JSON document.
func Parse(raw []byte) (SpecInput, error) {
	var spec SpecInput
	if err := json.Unmarshal(raw, &spec); err != nil {
		return SpecInput{}, fmt.Errorf("invalid specification input JSON: %w", err)
	}
	spec.Functions = nonemptyMaps(spec.Functions)
	spec.Webhooks = nonemptyMaps(spec.Webhooks)
	spec.Schemas = nonemptyMaps(spec.Schemas)
	return spec, nil
}

func nonemptyMaps(in []map[string]any) []map[string]any {
	if in == nil {
		return []map[string]any{}
	}
	return in
}

// Encode writes functions/webhooks/schemas as pretty JSON (no title).
func Encode(spec SpecInput) ([]byte, error) {
	payload := struct {
		Functions []map[string]any `json:"functions"`
		Webhooks  []map[string]any `json:"webhooks"`
		Schemas   []map[string]any `json:"schemas"`
	}{
		Functions: nonemptyMaps(spec.Functions),
		Webhooks:  nonemptyMaps(spec.Webhooks),
		Schemas:   nonemptyMaps(spec.Schemas),
	}
	return json.MarshalIndent(payload, "", "  ")
}

func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func resourceLabel(m map[string]any) string {
	ctx := mapString(m, "context")
	name := mapString(m, "name")
	if ctx == "" {
		return name
	}
	if name == "" {
		return ctx
	}
	return ctx + "." + name
}

func schemaContextName(m map[string]any) string {
	parts := make([]string, 0, 2)
	if ctx := mapString(m, "context"); ctx != "" {
		parts = append(parts, ctx)
	}
	if name := mapString(m, "name"); name != "" {
		parts = append(parts, name)
	}
	return strings.Join(parts, ".")
}
