package action

import (
	"context"
	"errors"
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

// NameLocalization joins the action's transaction; refresh runs only after commit.
type NameLocalization interface {
	WriteActionNames(context.Context, string, string, map[string]string) (int64, error)
	RefreshActionNames(context.Context, int64)
}

func (m *Module) SetNameLocalization(localization NameLocalization) {
	m.nameLocalization = localization
}

func (m *Module) writeNames(ctx context.Context, oldWord, word string, names map[string]string) (int64, error) {
	if names == nil && (oldWord == "" || oldWord == word || m.nameLocalization == nil) {
		return 0, nil
	}
	if m.nameLocalization == nil {
		return 0, errors.New("action name localization is unavailable")
	}
	return m.nameLocalization.WriteActionNames(ctx, oldWord, word, names)
}

func (m *Module) refreshNames(ctx context.Context, dictionaryID int64, value *Action) {
	if dictionaryID == 0 {
		return
	}
	m.nameLocalization.RefreshActionNames(ctx, dictionaryID)
	catalog := m.catalogSource.Current()
	value.localizeName(catalog, i18n.Locale(ctx, ""))
	value.loadNames(catalog)
}

// Names follow the stored bindings, because an action word is an editable identity.
// Prepare the locale maps once per read so lists and their filters share a snapshot.
type actionNameLabels struct {
	actions, resources, operations map[string]string
	separator                      string
}

func newActionNameLabels(catalog localization.Catalog, locale string) actionNameLabels {
	separator := " "
	if strings.HasPrefix(locale, "zh") {
		separator = ""
	}
	return actionNameLabels{
		actions:    catalog.Labels(locale, "actions"),
		resources:  catalog.Labels(locale, "resources"),
		operations: catalog.Labels(locale, "operations"),
		separator:  separator,
	}
}

func (labels actionNameLabels) name(word string, config Config) string {
	if name := labels.actions[word]; name != "" {
		return name
	}
	bindings, _ := configBindings(config)
	names := make([]string, 0, len(bindings))
	seen := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		resource, operation := labels.resources[binding.ResourceKey], labels.operations[binding.Operation]
		if resource == "" || operation == "" {
			return word
		}
		name := operation + labels.separator + resource
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	if len(names) == 0 {
		return word
	}
	return strings.Join(names, " / ")
}

func (value *Action) localizeName(catalog localization.Catalog, locale string) {
	value.Name = newActionNameLabels(catalog, locale).name(value.Word, value.Config)
}

// Keyword filters need each action's real bindings, including custom words. Other
// filters are applied first, and requests without name/keyword filters skip this read.
func (m *Module) nameFilterLabels(ctx context.Context, executor db.SQLExecutor, input ListActionsInput, labels actionNameLabels, groupLabels map[string]string) (map[string]string, error) {
	input.Q, input.Name = "", ""
	where, args := actionFilters(input, nil, groupLabels)
	rows, err := executor.QueryContext(ctx, "SELECT word, config - 'ui' - 'endpoints' FROM t_action"+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var word string
		var config Config
		if err := rows.Scan(&word, &config); err != nil {
			return nil, err
		}
		if name := labels.name(word, config); name != word {
			result[word] = name
		}
	}
	return result, rows.Err()
}

func (value *Action) loadNames(catalog localization.Catalog) {
	value.Names = map[string]string{}
	for _, locale := range catalog.Locales {
		actions, _ := catalog.Messages[locale.Tag].Frontend["actions"].(map[string]any)
		name, _ := actions[value.Word].(string)
		value.Names[locale.Tag] = name
	}
}
