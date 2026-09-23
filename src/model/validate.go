package model

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/polyapi/polyglot/src/api"
)

// DuplicateError is two functions or webhooks sharing the same context+name.
type DuplicateError struct {
	Identifier string
}

func (e *DuplicateError) Error() string {
	return "duplicated name and context: " + e.Identifier
}

// Validate reads a specification input file and POSTs each DTO to the platform validator.
func Validate(client *api.HTTPClient, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotExist
		}
		return err
	}
	if !jsonHasKey(raw, "functions") && !jsonHasKey(raw, "webhooks") {
		return fmt.Errorf("expected specification to contain \"webhooks\" and/or \"functions\", but found neither")
	}
	spec, err := Parse(raw)
	if err != nil {
		return err
	}
	return ValidateSpec(client, spec)
}

func jsonHasKey(raw []byte, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// ValidateSpec runs local duplicate checks then HTTP DTO validation.
func ValidateSpec(client *api.HTTPClient, spec SpecInput) error {
	if err := checkDuplicates(spec.Functions); err != nil {
		return err
	}
	if err := checkDuplicates(spec.Webhooks); err != nil {
		return err
	}
	if client == nil {
		return fmt.Errorf("missing API client")
	}
	for _, fn := range spec.Functions {
		if _, err := client.PostQuery("specification-input/validation/api-function", nil, fn); err != nil {
			return err
		}
	}
	for _, wh := range spec.Webhooks {
		if _, err := client.PostQuery("specification-input/validation/webhook-handle", nil, wh); err != nil {
			return err
		}
	}
	return nil
}

func checkDuplicates(resources []map[string]any) error {
	if id := duplicatedIdentifier(resources); id != "" {
		return &DuplicateError{Identifier: id}
	}
	return nil
}

func duplicatedIdentifier(resources []map[string]any) string {
	for i, first := range resources {
		name := mapString(first, "name")
		if name == "" {
			continue
		}
		ctx := mapString(first, "context")
		for j, second := range resources {
			if i == j {
				continue
			}
			if ctx == mapString(second, "context") && name == mapString(second, "name") {
				if ctx == "" {
					return name
				}
				return ctx + "." + name
			}
		}
	}
	return ""
}

// Load reads a specification input JSON file.
func Load(path string) (SpecInput, error) {
	return readSpecFile(path)
}

func readSpecFile(path string) (SpecInput, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SpecInput{}, ErrNotExist
		}
		return SpecInput{}, err
	}
	return Parse(raw)
}
