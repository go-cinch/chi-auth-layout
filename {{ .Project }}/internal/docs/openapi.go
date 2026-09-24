package docs

import (
	"fmt"

	"github.com/knadh/koanf/parsers/yaml"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
)

// Render server URLs and pagination bounds from runtime configuration.
func renderOpenAPI(data []byte, servers []config.HTTPDocsServersItemConfig, configured ...pagination.Limits) ([]byte, error) {
	parser := yaml.Parser()
	document, err := parser.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("decode openapi document: %w", err)
	}
	entries := make([]any, 0, len(servers))
	for _, item := range servers {
		entries = append(entries, map[string]any{"url": item.URL, "description": item.Description})
	}
	document["servers"] = entries
	if len(configured) > 0 {
		limits, err := pagination.New(int(configured[0].MaxP), int(configured[0].MaxS))
		if err != nil {
			return nil, err
		}
		applyPagination(document, limits)
	}
	output, err := parser.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode openapi document: %w", err)
	}
	return output, nil
}

// Update only schemas carrying a docsgen pagination marker.
func applyPagination(node any, limits pagination.Limits) {
	switch value := node.(type) {
	case map[string]any:
		switch value["x-pagination"] {
		case "p":
			value["description"] = fmt.Sprintf("Values outside 1..%d return an empty list.", limits.MaxP)
			value["default"] = 1
		case "s":
			value["description"] = fmt.Sprintf("Values outside 1..%d return an empty list.", limits.MaxS)
			value["default"] = int(limits.DefaultSize())
		}
		for _, child := range value {
			applyPagination(child, limits)
		}
	case []any:
		for _, child := range value {
			applyPagination(child, limits)
		}
	}
}
