package tests

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
	"{{ .Computed.module_name_final }}/internal/modules/usergroup"
)

func TestListsBatchRelationsAndEffectiveLockStatus(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO t_action (id,name,action_group,code,word) VALUES (101,'Read','Batch','BATCHRD1','batch.read'),(102,'Write','Batch','BATCHWR1','batch.write')`)
	exec(`INSERT INTO t_role (id,name,word,action) VALUES (101,'Batch editor','batch.editor','BATCHRD1,BATCHWR1'),(102,'Batch empty','batch.empty','')`)
	past, future := time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(time.Hour).UnixMilli()
	for i, metadata := range []string{fmt.Sprintf(`{"lock_expired_at":%d,"source":"test"}`, past), `{"lock_expired_at":0}`, fmt.Sprintf(`{"lock_expired_at":%d}`, future), `{}`, `{}`} {
		status := user.StatusLocked
		if i == 3 {
			status = user.StatusActive
		}
		if i == 4 {
			status = user.StatusPending
		}
		exec(`INSERT INTO t_user (id,username,code,password,role_id,action,status,metadata) VALUES ($1,$2,$3,'test-hash',101,'BATCHWR1,BATCHRD1,MISSING1',$4,$5)`, 101+i, fmt.Sprintf("batch_user_%d", i), fmt.Sprintf("BATCHU%02d", i), status, metadata)
	}
	exec(`INSERT INTO t_user_group (id,name,word,action) VALUES (101,'Batch first','batch.first','BATCHWR1,BATCHRD1'),(102,'Batch second','batch.second','BATCHRD1'),(103,'Batch empty','batch.empty','')`)
	exec(`INSERT INTO t_user_user_group_relation (user_id,user_group_id) VALUES (103,101),(101,101),(101,102)`)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false)
	roles := role.New(store, limits, actions, false)
	users := user.New(store, limits, nil, nil, actions, roles, auth.Switches{PasswordResetRequired: true})
	groups := usergroup.New(store, limits, actions, users)
	// PostgreSQL rejects any attempted UPDATE here, including an accidental global unlock.
	err := store.Tx(ctx, func(ctx context.Context) error {
		if _, err := store.SQL(ctx).ExecContext(ctx, `SET TRANSACTION READ ONLY`); err != nil {
			return err
		}
		size := int32(50)
		cases := []struct {
			statuses []int16
			ids      []int64
		}{
			{nil, []int64{105, 104, 103, 102, 101}},
			{[]int16{user.StatusActive}, []int64{104, 101}},
			{[]int16{user.StatusLocked}, []int64{103, 102}},
			{[]int16{user.StatusPending}, []int64{105}},
			{[]int16{user.StatusActive, user.StatusLocked}, []int64{104, 103, 102, 101}},
		}
		for _, tc := range cases {
			result, err := users.List(ctx, user.ListUsersInput{Username: "batch_user_", Statuses: tc.statuses, PageSize: &size})
			if err != nil {
				return err
			}
			got := make([]int64, 0, len(result.Items))
			for _, item := range result.Items {
				got = append(got, item.ID)
				if len(item.Actions) != 2 || item.Actions[0].Code != "BATCHWR1" || item.Actions[1].Code != "BATCHRD1" || item.Role == nil || len(item.Role.Actions) != 2 || item.Role.Actions[0].Code != "BATCHRD1" {
					t.Fatalf("user relations: %#v", item)
				}
				if item.ID == 101 {
					if _, exists := item.Metadata["lock_expired_at"]; exists || item.Status != user.StatusActive || item.Metadata["source"] != "test" {
						t.Fatalf("expired user: %#v", item)
					}
				}
			}
			if result.Total != int64(len(tc.ids)) || !reflect.DeepEqual(got, tc.ids) {
				t.Fatalf("statuses %v: total %d, ids %v; want %v", tc.statuses, result.Total, got, tc.ids)
			}
		}
		page, two := int32(2), int32(1)
		paged, err := users.List(ctx, user.ListUsersInput{Username: "batch_user_", Statuses: []int16{user.StatusActive}, Page: &page, PageSize: &two})
		if err != nil {
			return err
		}
		if paged.Total != 2 || len(paged.Items) != 1 || paged.Items[0].ID != 101 {
			t.Fatalf("filtered page: %#v", paged)
		}
		page = 3
		paged, err = users.List(ctx, user.ListUsersInput{Username: "batch_user_", Statuses: []int16{user.StatusActive}, Page: &page, PageSize: &two})
		if err != nil {
			return err
		}
		if paged.Total != 2 || len(paged.Items) != 0 {
			t.Fatalf("past last page: %#v", paged)
		}
		rs, err := roles.List(ctx, role.ListRolesInput{Word: "batch.", PageSize: &size})
		if err != nil {
			return err
		}
		if rs.Total != 2 || len(rs.Items) != 2 || rs.Items[0].ID != 102 || rs.Items[0].Actions == nil || len(rs.Items[1].Actions) != 2 {
			t.Fatalf("roles: %#v", rs)
		}
		gs, err := groups.List(ctx, usergroup.ListUserGroupsInput{Word: "batch.", PageSize: &size})
		if err != nil {
			return err
		}
		if gs.Total != 3 || len(gs.Items) != 3 || gs.Items[0].Users == nil || len(gs.Items[0].Users) != 0 || len(gs.Items[1].Users) != 1 || len(gs.Items[2].Users) != 2 || gs.Items[2].Users[0].ID != 101 || gs.Items[2].Users[1].ID != 103 || gs.Items[2].Actions[0].Code != "BATCHWR1" {
			t.Fatalf("groups: %#v", gs)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var status int16
	var hasLock bool
	if err := store.DB.QueryRowContext(ctx, `SELECT status,metadata ? 'lock_expired_at' FROM t_user WHERE id=101`).Scan(&status, &hasLock); err != nil {
		t.Fatal(err)
	}
	if status != user.StatusLocked || !hasLock {
		t.Fatal("list persisted an unlock")
	}
}
