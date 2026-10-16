package tests

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/report"
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

		selected, err := users.List(ctx, user.ListUsersInput{Username: "batch_user_", Statuses: []int16{user.StatusActive}, PageSize: &size, Fields: report.Fields{"id", "status", "metadata", "role_summary"}})
		if err != nil {
			return err
		}
		if selected.Total != 2 || len(selected.Items) != 2 {
			t.Fatalf("selected page: %#v", selected)
		}
		for _, item := range selected.Items {
			if item.Status != user.StatusActive || item.RoleSummary == nil || item.RoleSummary.Name != "Batch editor" || item.Actions != nil || item.Role != nil {
				t.Fatalf("selected user: %#v", item)
			}
			if item.ID == 101 {
				if _, exists := item.Metadata["lock_expired_at"]; exists || item.Metadata["source"] != "test" {
					t.Fatalf("selected expiry: %#v", item)
				}
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

func TestRoleExactPermissionGroupPostgres(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exact := "Finance, 100%_'"
	exec(`INSERT INTO t_action (id,name,action_group,code,word) VALUES
		(101,'Read',$1,'GROUPRD1','group.read'),(102,'Write',$1,'GROUPWR1','group.write'),
		(103,'Similar',$2,'GROUPSM1','group.similar'),(104,'Other','Other','GROUPUN1','group.other')`, exact, exact+" extra")
	exec(`INSERT INTO t_role (id,name,word,action) VALUES
		(101,'Group editor','group.editor',' GROUPWR1, GROUPRD1, GROUPRD1, MISSING1 '),
		(102,'Group reader','group.reader','GROUPRD1'),
		(103,'Group similar','group.similar','GROUPSM1'),
		(104,'Group other','group.other','GROUPUN1'),
		(105,'Group orphan','group.orphan','MISSING1'),
		(106,'Group empty','group.empty','')`)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false)
	roles := role.New(store, limits, actions, false)
	size, one, two, three := int32(100), int32(1), int32(2), int32(3)
	empty, partial, unknown := "", "Finance", "Missing"
	assigned, unassigned := true, false
	cases := []struct {
		name  string
		input role.ListRolesInput
		total int64
		ids   []int64
	}{
		{"all", role.ListRolesInput{}, 6, []int64{106, 105, 104, 103, 102, 101}},
		{"exact, duplicate codes and multiple matching actions", role.ListRolesInput{GroupExact: &exact}, 2, []int64{102, 101}},
		{"explicit empty does not clear filter", role.ListRolesInput{GroupExact: &empty}, 0, []int64{}},
		{"partial group does not match", role.ListRolesInput{GroupExact: &partial}, 0, []int64{}},
		{"unknown group", role.ListRolesInput{GroupExact: &unknown}, 0, []int64{}},
		{"name and action code", role.ListRolesInput{GroupExact: &exact, Name: "editor", ActionCode: "GROUPWR1"}, 1, []int64{101}},
		{"assigned group", role.ListRolesInput{GroupExact: &exact, HasPermissions: &assigned}, 2, []int64{102, 101}},
		{"unassigned cannot match group", role.ListRolesInput{GroupExact: &exact, HasPermissions: &unassigned}, 0, []int64{}},
		{"unassigned includes orphan and empty", role.ListRolesInput{HasPermissions: &unassigned}, 2, []int64{106, 105}},
		{"second page", role.ListRolesInput{GroupExact: &exact, Page: &two, PageSize: &one}, 2, []int64{101}},
		{"past last page", role.ListRolesInput{GroupExact: &exact, Page: &three, PageSize: &one}, 2, []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			input.Word = "group."
			if input.PageSize == nil {
				input.PageSize = &size
			}
			result, err := roles.List(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int64, 0, len(result.Items))
			for _, item := range result.Items {
				ids = append(ids, item.ID)
				if item.ID == 101 && (len(item.Actions) != 2 || item.Actions[0].Code != "GROUPWR1" || item.Actions[1].Code != "GROUPRD1") {
					t.Fatalf("complete ordered role permissions: %#v", item)
				}
			}
			if result.Total != tc.total || !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("total %d ids %v, want %d %v", result.Total, ids, tc.total, tc.ids)
			}
		})
	}
}

func TestRoleAllPermissionGroupsPostgres(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	finance, operations, audit := "Finance, 100%_'", "Operations", "Audit"
	exec(`INSERT INTO t_action (id,name,action_group,code,word) VALUES
		(101,'Finance read',$1,'ALLFINR1','all.finance.read'),(102,'Finance write',$1,'ALLFINW1','all.finance.write'),
		(103,'Operations',$2,'ALLOPSR1','all.operations'),(104,'Audit',$3,'ALLAUDR1','all.audit'),
		(105,'Extra group','Extra','ALLEXT01','all.extra'),(106,'Spaced operations',' Operations ','ALLSPA01','all.spaced'),
		(107,'Partial finance','Finance','ALLPAR01','all.partial')`, finance, operations, audit)
	exec(`INSERT INTO t_role (id,name,word,action) VALUES
		(101,'All three first','all.first',' ALLFINW1, ALLOPSR1, ALLAUDR1, ALLFINR1, ALLFINW1, ALLEXT01, MISSING1 '),
		(102,'All three second','all.second','ALLFINR1,ALLOPSR1,ALLAUDR1'),
		(103,'Finance operations','all.finance.operations','ALLFINR1,ALLOPSR1'),
		(104,'Finance audit','all.finance.audit','ALLFINR1,ALLAUDR1'),
		(105,'Operations audit','all.operations.audit','ALLOPSR1,ALLAUDR1'),
		(106,'Finance only','all.finance.only','ALLFINR1'),
		(107,'Empty role','all.empty',''),(108,'Orphan role','all.orphan','MISSING1'),
		(109,'Spaced group','all.spaced','ALLFINR1,ALLSPA01,ALLAUDR1'),
		(110,'Partial finance','all.partial','ALLPAR01,ALLOPSR1,ALLAUDR1')`)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false)
	roles := role.New(store, limits, actions, false)
	size, one, two, three := int32(100), int32(1), int32(2), int32(3)
	empty, extra := "", "Extra"
	assigned, unassigned := true, false
	cases := []struct {
		name  string
		input role.ListRolesInput
		total int64
		ids   []int64
	}{
		{"omitted", role.ListRolesInput{}, 10, []int64{110, 109, 108, 107, 106, 105, 104, 103, 102, 101}},
		{"empty array clears group restriction", role.ListRolesInput{GroupsAll: []string{}}, 10, []int64{110, 109, 108, 107, 106, 105, 104, 103, 102, 101}},
		{"three groups require every group", role.ListRolesInput{GroupsAll: []string{finance, operations, audit}}, 2, []int64{102, 101}},
		{"remove audit condition", role.ListRolesInput{GroupsAll: []string{finance, operations}}, 3, []int64{103, 102, 101}},
		{"remove operations condition", role.ListRolesInput{GroupsAll: []string{finance, audit}}, 4, []int64{109, 104, 102, 101}},
		{"duplicate selections and actions do not duplicate roles", role.ListRolesInput{GroupsAll: []string{finance, operations, audit, finance, operations}}, 2, []int64{102, 101}},
		{"commas are literal", role.ListRolesInput{GroupsAll: []string{"Finance", operations, audit}}, 1, []int64{110}},
		{"empty group does not clear filter", role.ListRolesInput{GroupsAll: []string{""}}, 0, []int64{}},
		{"whitespace group does not clear filter", role.ListRolesInput{GroupsAll: []string{" "}}, 0, []int64{}},
		{"group spaces are preserved", role.ListRolesInput{GroupsAll: []string{finance, " Operations ", audit}}, 1, []int64{109}},
		{"case sensitive", role.ListRolesInput{GroupsAll: []string{finance, "operations", audit}}, 0, []int64{}},
		{"unknown group excludes all", role.ListRolesInput{GroupsAll: []string{finance, operations, "Missing"}}, 0, []int64{}},
		{"legacy condition also required", role.ListRolesInput{GroupExact: &extra, GroupsAll: []string{finance, operations, audit}}, 1, []int64{101}},
		{"empty legacy condition is retained", role.ListRolesInput{GroupExact: &empty, GroupsAll: []string{finance, operations, audit}}, 0, []int64{}},
		{"all other filters compose", role.ListRolesInput{GroupsAll: []string{finance, operations, audit}, Name: "first", ActionCode: "ALLFINW1", HasPermissions: &assigned}, 1, []int64{101}},
		{"unassigned cannot match groups", role.ListRolesInput{GroupsAll: []string{finance, operations, audit}, HasPermissions: &unassigned}, 0, []int64{}},
		{"unassigned includes empty and orphan roles", role.ListRolesInput{GroupsAll: []string{}, HasPermissions: &unassigned}, 2, []int64{108, 107}},
		{"second page uses matching count", role.ListRolesInput{GroupsAll: []string{finance, operations, audit}, Page: &two, PageSize: &one}, 2, []int64{101}},
		{"past last page keeps matching count", role.ListRolesInput{GroupsAll: []string{finance, operations, audit}, Page: &three, PageSize: &one}, 2, []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			input.Word = "all."
			if input.PageSize == nil {
				input.PageSize = &size
			}
			result, err := roles.List(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int64, 0, len(result.Items))
			for _, item := range result.Items {
				ids = append(ids, item.ID)
				if item.ID == 101 && (len(item.Actions) != 5 || item.Actions[0].Code != "ALLFINW1" || item.Actions[4].Code != "ALLEXT01") {
					t.Fatalf("complete ordered role permissions: %#v", item)
				}
			}
			if result.Total != tc.total || !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("total %d ids %v, want %d %v", result.Total, ids, tc.total, tc.ids)
			}
		})
	}
}

func TestRoleKeywordPostgres(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	_, err := store.DB.ExecContext(ctx, `INSERT INTO t_role (id,name,word,action) VALUES
		(101,'Needle name','keyword.first',''),
		(102,'Other','keyword.needle',''),
		(103,'Needle both','keyword.needle.both',''),
		(104,'Literal 100%_!','keyword.literal',''),
		(105,'Literal 100xyz','keyword.decoy',''),
		(106,'Comma a,b','keyword.comma','')`)
	if err != nil {
		t.Fatal(err)
	}
	limits, _ := pagination.New(100, 100)
	roles := role.New(store, limits, action.New(store, limits, false), false)
	size, one, two, four := int32(100), int32(1), int32(2), int32(4)
	cases := []struct {
		name  string
		input role.ListRolesInput
		total int64
		ids   []int64
	}{
		{"name or word and deduplicate", role.ListRolesInput{Q: "  nEeDlE  "}, 3, []int64{103, 102, 101}},
		{"keyword AND name", role.ListRolesInput{Q: "needle", Name: "Other"}, 1, []int64{102}},
		{"keyword AND word", role.ListRolesInput{Q: "needle", Word: "keyword.first"}, 1, []int64{101}},
		{"literal wildcards", role.ListRolesInput{Q: "100%_!"}, 1, []int64{104}},
		{"comma is literal", role.ListRolesInput{Q: "a,b"}, 1, []int64{106}},
		{"blank", role.ListRolesInput{Q: " ", Word: "keyword."}, 6, []int64{106, 105, 104, 103, 102, 101}},
		{"no match", role.ListRolesInput{Q: "unknown"}, 0, []int64{}},
		{"shared pagination", role.ListRolesInput{Q: "needle", Page: &two, PageSize: &one}, 3, []int64{102}},
		{"past last page", role.ListRolesInput{Q: "needle", Page: &four, PageSize: &one}, 3, []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			if input.PageSize == nil {
				input.PageSize = &size
			}
			result, err := roles.List(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int64, 0, len(result.Items))
			for _, item := range result.Items {
				ids = append(ids, item.ID)
			}
			if result.Total != tc.total || !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("total %d ids %v, want %d %v", result.Total, ids, tc.total, tc.ids)
			}
		})
	}
}

func TestActionKeywordPostgres(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	_, err := store.DB.ExecContext(ctx, `INSERT INTO t_action (id,name,word,code,action_group) VALUES
		(101,'QMatch name','qtest.first','QKEY0001','Keyword'),
		(102,'Other','qtest.qmatch','QKEY0002','Keyword'),
		(103,'Code only','qtest.third','QMATCH03','Other'),
		(104,'QMatch both','qtest.qmatch.both','QMATCH04','Keyword'),
		(105,'Literal 100%_!','qtest.literal','QKEY0005','Keyword'),
		(106,'Literal 100xyz','qtest.decoy','QKEY0006','Keyword')`)
	if err != nil {
		t.Fatal(err)
	}
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false)
	size, one, two, five := int32(100), int32(1), int32(2), int32(5)
	group := "Keyword"
	cases := []struct {
		name  string
		input action.ListActionsInput
		total int64
		ids   []int64
	}{
		{"name OR word OR code", action.ListActionsInput{Q: "  qMaTcH  "}, 4, []int64{104, 103, 102, 101}},
		{"AND group", action.ListActionsInput{Q: "qmatch", GroupExact: &group}, 3, []int64{104, 102, 101}},
		{"AND name", action.ListActionsInput{Q: "qmatch", Name: "Other"}, 1, []int64{102}},
		{"AND code", action.ListActionsInput{Q: "qmatch", Code: "QKEY0001"}, 1, []int64{101}},
		{"literal wildcards", action.ListActionsInput{Q: "100%_!"}, 1, []int64{105}},
		{"blank", action.ListActionsInput{Q: " "}, 6, []int64{106, 105, 104, 103, 102, 101}},
		{"missing", action.ListActionsInput{Q: "missing"}, 0, []int64{}},
		{"second page", action.ListActionsInput{Q: "qmatch", Page: &two, PageSize: &one}, 4, []int64{103}},
		{"past last page", action.ListActionsInput{Q: "qmatch", Page: &five, PageSize: &one}, 4, []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			input.Word = "qtest."
			if input.PageSize == nil {
				input.PageSize = &size
			}
			result, err := actions.List(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int64, 0, len(result.Items))
			for _, item := range result.Items {
				ids = append(ids, item.ID)
			}
			if result.Total != tc.total || !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("total %d ids %v, want %d %v", result.Total, ids, tc.total, tc.ids)
			}
		})
	}
}

func TestUserKeywordPostgres(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	_, err := store.DB.ExecContext(ctx, `INSERT INTO t_user (id,username,code,password,status) VALUES
		(101,'qtest_qmatch','UKEY0001','hash',1),
		(102,'qtest_other','QMATCH02','hash',0),
		(103,'qtest_qmatch_both','QMATCH03','hash',1),
		(104,'qtest_100%_!','UKEY0004','hash',1),
		(105,'qtest_100xyz','UKEY0005','hash',1)`)
	if err != nil {
		t.Fatal(err)
	}
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false)
	users := user.New(store, limits, nil, nil, actions, role.New(store, limits, actions, false), auth.Switches{})
	size, one, two, four := int32(100), int32(1), int32(2), int32(4)
	cases := []struct {
		name  string
		input user.ListUsersInput
		total int64
		ids   []int64
	}{
		{"username OR code", user.ListUsersInput{Q: "  qMaTcH  "}, 3, []int64{103, 102, 101}},
		{"AND status", user.ListUsersInput{Q: "qmatch", Statuses: []int16{user.StatusActive}}, 2, []int64{103, 101}},
		{"AND username", user.ListUsersInput{Q: "qmatch", Username: "qtest_other"}, 1, []int64{102}},
		{"AND code", user.ListUsersInput{Q: "qmatch", Code: "UKEY0001"}, 1, []int64{101}},
		{"literal wildcards", user.ListUsersInput{Q: "100%_!"}, 1, []int64{104}},
		{"blank", user.ListUsersInput{Q: " "}, 5, []int64{105, 104, 103, 102, 101}},
		{"missing", user.ListUsersInput{Q: "missing"}, 0, []int64{}},
		{"second page", user.ListUsersInput{Q: "qmatch", Page: &two, PageSize: &one}, 3, []int64{102}},
		{"past last page", user.ListUsersInput{Q: "qmatch", Page: &four, PageSize: &one}, 3, []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			if input.Username == "" {
				input.Username = "qtest_"
			}
			if input.PageSize == nil {
				input.PageSize = &size
			}
			result, err := users.List(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int64, 0, len(result.Items))
			for _, item := range result.Items {
				ids = append(ids, item.ID)
			}
			if result.Total != tc.total || !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("total %d ids %v, want %d %v", result.Total, ids, tc.total, tc.ids)
			}
		})
	}
}
