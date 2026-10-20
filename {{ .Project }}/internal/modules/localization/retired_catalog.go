package localization

import (
	"encoding/json"
	"errors"
)

// Retired policy fields must never be silently ignored on a catalog write.
// Migrations and the browser draft importer convert the old representation.
func rejectRetiredResourceFields(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var catalog struct {
		Defaults  json.RawMessage `json:"defaults"`
		Resources map[string]struct {
			Collection json.RawMessage `json:"collection"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return err
	}
	if catalog.Defaults != nil {
		return errors.New("resource_catalog.defaults is retired; use explicit action resource bindings")
	}
	for _, node := range catalog.Resources {
		if node.Collection != nil {
			return errors.New("resource collection is retired; use frontend_visible and reference_operations")
		}
	}
	return nil
}
