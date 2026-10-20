package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/code"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

// ResolveAssignment joins the owner mutation's transaction. Existing callers
// using codes keep their contract; resource selections never fall back to codes.
func ResolveAssignment(ctx context.Context, actions any, resource, operation string, bindings []ResourceBinding) ([]string, error) {
	resolver, ok := actions.(interface {
		ResolveAssignment(context.Context, string, string, []ResourceBinding) ([]string, error)
	})
	if !ok {
		return nil, errors.New("permission assignment resolver is unavailable")
	}
	return resolver.ResolveAssignment(ctx, resource, operation, bindings)
}

// ResolveAssignment must be called inside Store.Tx. The same permission lock
// protects authorization, the committed catalog, lookup, insertion and binding.
func (m *Module) ResolveAssignment(ctx context.Context, resource, operation string, bindings []ResourceBinding) ([]string, error) {
	if (resource != "user" && resource != "role" && resource != "user-group") || (operation != "create" && operation != "update") {
		return nil, ErrInvalid
	}
	if err := authn.RequirePermissionOperation(ctx, m.administration, resource, operation); err != nil {
		return nil, err
	}
	executor := m.store.SQL(ctx)
	if _, err := executor.ExecContext(ctx, `SELECT pg_advisory_xact_lock(73482301991337)`); err != nil {
		return nil, err
	}
	if m.definitions == nil {
		return nil, errors.New("action definition catalog is unavailable")
	}
	catalog, err := m.definitions.ActionCatalog(ctx)
	if err != nil {
		return nil, err
	}
	// Assignment input is always explicit. null must never mean "all features".
	for _, binding := range bindings {
		if binding.FeatureKeys == nil {
			return nil, ErrInvalid
		}
	}
	selected, err := normalizedAssignment(catalog, bindings)
	if err != nil {
		return nil, err
	}
	codes := []string{}
	if len(selected) == 0 {
		return codes, nil
	}
	scopes := []string{}
	for _, binding := range selected {
		scopes = append(scopes, strings.Split(binding.ResourceKey, ":")[0])
	}
	candidates, err := assignmentCandidates(ctx, executor, NormalizeCodes(scopes))
	if err != nil {
		return nil, err
	}
	// Only a complete, single resource/operation definition may satisfy an
	// atomic selection. A multi-resource or broader-feature code is not a match.
	matching := map[string]string{}
	for _, candidate := range candidates {
		known, err := assignmentBindings(catalog, candidate.Word, candidate.Config)
		if err != nil || len(known) != 1 {
			continue
		}
		signature := assignmentSignature(known[0])
		if matching[signature] == "" {
			matching[signature] = candidate.Code
		}
	}
	for _, binding := range selected {
		signature := assignmentSignature(binding)
		matched := matching[signature]
		if matched == "" {
			matched, err = m.createAssignmentAction(ctx, catalog, binding)
			if err != nil {
				return nil, err
			}
			matching[signature] = matched
		}
		codes = append(codes, matched)
	}
	return NormalizeCodes(codes), nil
}

func normalizedAssignment(catalog localization.Catalog, bindings []ResourceBinding) ([]ResourceBinding, error) {
	result := make([]ResourceBinding, 0, len(bindings))
	seen := map[string]bool{}
	for _, binding := range bindings {
		scope := strings.Split(binding.ResourceKey, ":")[0]
		identity := binding.ResourceKey + ":" + binding.Operation
		if !validResourceBinding(scope+":read:assignment:resource", binding) || seen[identity] {
			return nil, ErrInvalid
		}
		seen[identity] = true
		if _, err := catalog.ResolveResourceBinding(scope, binding.ResourceKey, binding.Operation, binding.FeatureKeys); err != nil {
			return nil, ErrInvalid
		}
		features := binding.FeatureKeys
		if features == nil {
			features = catalog.ResourceCatalog.Resources[binding.ResourceKey].Operations[binding.Operation].ButtonKeys
		}
		binding.FeatureKeys = append([]string{}, features...)
		sort.Strings(binding.FeatureKeys)
		result = append(result, binding)
	}
	sort.Slice(result, func(i, j int) bool { return assignmentSignature(result[i]) < assignmentSignature(result[j]) })
	return result, nil
}

func assignmentSignature(binding ResourceBinding) string {
	encoded, _ := json.Marshal(binding)
	return string(encoded)
}

func assignmentBindings(catalog localization.Catalog, word string, config Config) ([]ResourceBinding, error) {
	if bindings, explicit := configBindings(config); explicit {
		return normalizedAssignment(catalog, bindings)
	}
	scope := strings.Split(word, ":")[0]
	if config.Builtin == "all" {
		keys := []string{}
		for key, node := range catalog.ResourceCatalog.Resources {
			if strings.HasPrefix(key, scope+":") && node.Kind == "resource" && node.PermissionScope == "*" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			return normalizedAssignment(catalog, []ResourceBinding{
				{ResourceKey: keys[0], Operation: "*", FeatureKeys: []string{}},
			})
		}
	}
	if config.Builtin == "default" && config.ResourceKeys != nil {
		bindings := []ResourceBinding{}
		for _, key := range *config.ResourceKeys {
			var features []string
			if config.FeatureKeys != nil {
				features = []string{}
				for _, allowed := range catalog.ResourceCatalog.Resources[key].Operations["read"].ButtonKeys {
					for _, selected := range config.FeatureKeys {
						if allowed == selected {
							features = append(features, allowed)
						}
					}
				}
			}
			bindings = append(bindings, ResourceBinding{ResourceKey: key, Operation: "read", FeatureKeys: features})
		}
		return normalizedAssignment(catalog, bindings)
	}
	return nil, ErrInvalid
}

func assignmentCandidates(ctx context.Context, executor db.SQLExecutor, scopes []string) ([]Action, error) {
	rows, err := executor.QueryContext(ctx, `SELECT code, word, config - 'ui' - 'endpoints' FROM t_action WHERE scope = ANY($1) ORDER BY id`, pq.Array(scopes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Action{}
	for rows.Next() {
		var action Action
		if err := rows.Scan(&action.Code, &action.Word, &action.Config); err != nil {
			return nil, err
		}
		result = append(result, action)
	}
	return result, rows.Err()
}

func (m *Module) createAssignmentAction(ctx context.Context, catalog localization.Catalog, binding ResourceBinding) (string, error) {
	scope := strings.Split(binding.ResourceKey, ":")[0]
	group := ""
	groups := catalog.GroupKeys()
	visited := map[string]bool{}
	for key := binding.ResourceKey; key != "" && !visited[key]; key = catalog.ResourceCatalog.Resources[key].Parent {
		visited[key] = true
		node := catalog.ResourceCatalog.Resources[key]
		if groups[node.PermissionGroup] {
			group = node.PermissionGroup
			break
		}
		if groups[key] {
			group = key
			break
		}
	}
	if group == "" {
		available := []string{}
		for key := range groups {
			if strings.HasPrefix(key, scope+":") {
				available = append(available, key)
			}
		}
		sort.Strings(available)
		if len(available) == 0 {
			return "", ErrInvalid
		}
		group = available[0]
	}
	operation := binding.Operation
	if operation == "*" {
		operation = "all"
	}
	for range 5 {
		actionCode, err := code.Generate()
		if err != nil {
			return "", err
		}
		word := scope + ":" + operation + ":assigned:p" + strings.ToLower(actionCode)
		bindings := []ResourceBinding{binding}
		config := Config{Bindings: &bindings, UI: []UIBinding{}, Endpoints: []Endpoint{}}
		if err := resolveStoredConfig(catalog, word, &config); err != nil {
			return "", err
		}
		encoded, err := json.Marshal(config)
		if err != nil {
			return "", err
		}
		now := time.Now().UnixMilli()
		result, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_action (created_at, updated_at, code, action_group, word, config) VALUES ($1,$1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, now, actionCode, group, word, string(encoded))
		if err != nil {
			return "", fmt.Errorf("create assignment action: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return "", err
		}
		if count == 1 {
			return actionCode, nil
		}
	}
	return "", errors.New("generate unique assignment action")
}
