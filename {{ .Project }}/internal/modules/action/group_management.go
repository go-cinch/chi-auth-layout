package action

import (
	"context"
	"errors"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

var (
	ErrGroupConflict = apperror.New("ACTION_GROUP_CONFLICT", "The permission groups have changed. Refresh and try again.")
	ErrGroupNotEmpty = apperror.New("ACTION_GROUP_NOT_EMPTY", "The permission group is not empty.")
)

type GroupChange struct {
	Operation     string            `json:"operation"`
	Word          string            `json:"word"`
	OriginalWord  string            `json:"original_word,omitempty"`
	Names         map[string]string `json:"names,omitempty"`
	ExpectedNames map[string]string `json:"expected_names,omitempty"`
}

type GroupOrder struct {
	Order         []string `json:"order"`
	ExpectedOrder []string `json:"expected_order"`
}

type GroupStorage interface {
	WriteActionGroup(context.Context, GroupChange) (int64, error)
	WriteActionGroupOrder(context.Context, GroupOrder) (int64, error)
	RefreshActionGroups(context.Context, int64)
}

func (m *Module) SetGroupStorage(storage GroupStorage) { m.groupStorage = storage }

func validGroupWord(word string) bool {
	parts := strings.Split(word, ":")
	if len(word) > 128 || len(parts) < 3 {
		return false
	}
	for _, part := range parts {
		if !segmentPattern.MatchString(part) {
			return false
		}
	}
	return true
}

func (m *Module) UpdateGroup(ctx context.Context, change GroupChange) error {
	if !validGroupWord(change.Word) {
		return ErrInvalid
	}
	control := ""
	switch change.Operation {
	case "create":
		if change.OriginalWord != "" || change.ExpectedNames != nil || len(change.Names) == 0 {
			return ErrInvalid
		}
		control = "create-group"
	case "update":
		if !validGroupWord(change.OriginalWord) || change.ExpectedNames == nil ||
			strings.SplitN(change.OriginalWord, ":", 2)[0] != strings.SplitN(change.Word, ":", 2)[0] {
			return ErrInvalid
		}
		control = "edit-group"
	case "delete":
		if change.OriginalWord != "" || change.Names != nil || change.ExpectedNames == nil {
			return ErrInvalid
		}
		control = "delete-group"
	default:
		return ErrInvalid
	}
	return m.mutateGroups(ctx, control, func(tx context.Context) (int64, error) {
		return m.groupStorage.WriteActionGroup(tx, change)
	})
}

func (m *Module) ReorderGroups(ctx context.Context, order GroupOrder) error {
	if order.Order == nil || order.ExpectedOrder == nil {
		return ErrInvalid
	}
	return m.mutateGroups(ctx, "reorder-groups", func(tx context.Context) (int64, error) {
		return m.groupStorage.WriteActionGroupOrder(tx, order)
	})
}

type groupControlPolicy interface {
	RequireActionGroupControl(context.Context, string) error
}

func (m *Module) mutateGroups(ctx context.Context, control string, write func(context.Context) (int64, error)) error {
	policy, ok := m.administration.(groupControlPolicy)
	if !ok {
		return authn.ErrPermissionGrantRequired
	}
	if m.groupStorage == nil {
		return errors.New("permission group storage is unavailable")
	}
	var id int64
	err := m.store.MutationTx(ctx, func(tx context.Context) error {
		if err := policy.RequireActionGroupControl(tx, control); err != nil {
			return err
		}
		var err error
		id, err = write(tx)
		if err == nil && id != 0 {
			MarkPermissionMutation(tx)
		}
		return err
	})
	if err == nil && id != 0 {
		m.groupStorage.RefreshActionGroups(ctx, id)
	}
	return err
}
