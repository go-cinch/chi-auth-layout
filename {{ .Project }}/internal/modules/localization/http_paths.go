package localization

import (
	"bytes"
	"encoding/json"
)

// CanonicalCatalog keeps persisted endpoint identities independent of the mount.
// Only HTTP transport mappings are rewritten; frontend paths and RPCs are untouched.
func (m *Module) CanonicalCatalog(raw []byte) ([]byte, error) {
	if len(m.httpPaths) == 0 {
		return raw, nil
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	var catalog map[string]any
	if value, ok := document["resource_catalog"]; !ok {
		return raw, nil
	} else {
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		if err := decoder.Decode(&catalog); err != nil {
			return nil, err
		}
	}
	changed := false
	rewrite := func(entries any) {
		items, _ := entries.([]any)
		for _, item := range items {
			value, _ := item.(map[string]any)
			previous, _ := value["path"].(string)
			if path, ok := m.canonicalHTTPPaths[previous]; ok && path != previous {
				value["path"] = path
				changed = true
			}
		}
	}
	buttons, _ := catalog["buttons"].(map[string]any)
	for _, item := range buttons {
		feature, _ := item.(map[string]any)
		rewrite(feature["http"])
	}
	if !changed {
		return raw, nil
	}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		return nil, err
	}
	document["resource_catalog"] = encoded
	return json.Marshal(document)
}

func rewriteHTTPPaths(catalog *ResourceCatalog, paths map[string]string) bool {
	changed := false
	for key, feature := range catalog.Buttons {
		for index := range feature.HTTP {
			if path, ok := paths[feature.HTTP[index].Path]; ok && path != feature.HTTP[index].Path {
				feature.HTTP[index].Path = path
				changed = true
			}
		}
		catalog.Buttons[key] = feature
	}
	return changed
}
