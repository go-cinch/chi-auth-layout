package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
	"{{ .Computed.module_name_final }}/internal/modules/usergroup"
)

func TestActionGRPCStorageAndListProjection(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	ctx := t.Context()
	for _, endpoints := range []string{
		`[{"protocol":"grpc","path":"/auth.v1.Dictionary/GetDictionary"}]`,
		`[{"protocol":"grpc","method":"","path":"/auth.v1.Dictionary/GetDictionary"}]`,
		`[{"protocol":"grpc","method":"GET","path":"/auth.v1.Dictionary/GetDictionary"}]`,
		`[{"protocol":"grpc","method":null,"path":"/auth.v1.Dictionary/GetDictionary"}]`,
		`[{"protocol":"grpc","method":[],"path":"/auth.v1.Dictionary/GetDictionary"}]`,
	} {
		if _, err := store.DB.ExecContext(ctx, `UPDATE t_action SET config = JSONB_SET(config, '{endpoints}', $1::JSONB) WHERE word = 'admin:read:system:dictionary'`, endpoints); err == nil {
			t.Fatalf("accepted invalid persisted gRPC method: %s", endpoints)
		}
	}
	endpoints := `[{"protocol":"http","method":"GET","path":"/dictionary/{id}"},{"protocol":"grpc","method":"GRPC","path":"/auth.v1.Dictionary/GetDictionary"}]`
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_action SET config = JSONB_SET(config, '{endpoints}', $1::JSONB) WHERE word = 'admin:read:system:dictionary'`, endpoints); err != nil {
		t.Fatal(err)
	}
	catalog := localizer.Current()
	for _, key := range catalog.ResourceCatalog.Resources["admin:system:dictionary"].Operations["read"].ButtonKeys {
		feature := catalog.ResourceCatalog.Buttons[key]
		feature.HTTP = []localization.HTTPPath{ {Method: "GET", Path: "/dictionary/{id}"}}
		feature.GRPC = []string{"/auth.v1.Dictionary/GetDictionary"}
		catalog.ResourceCatalog.Buttons[key] = feature
	}
	rawCatalog, _ := json.Marshal(catalog)
	if err := localizer.Install(rawCatalog); err != nil {
		t.Fatal(err)
	}
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false, localizer)
	result, err := actions.List(ctx, action.ListActionsInput{Word: "admin:read:system:dictionary", Fields: report.Fields{"resource"}})
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("list: %+v %v", result, err)
	}
	if got := result.Items[0].Resource; got != "GET|/dictionary/{id}\nGRPC|/auth.v1.Dictionary/GetDictionary" {
		t.Fatalf("resource projection: %q", got)
	}
}

func TestListsBatchRelationsAndEffectiveLockStatus(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO t_action (id,action_group,code,word,config) VALUES (101,'admin:batch:record','BATCHRD1','admin:read:batch:record','{"bindings":[],"ui":[],"endpoints":[]}'),(102,'admin:batch:record','BATCHWR1','admin:update:batch:record','{"bindings":[],"ui":[],"endpoints":[]}')`)
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
	actions := action.New(store, limits, false, localizer)
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
	localizer := testLocalization(t, store)
	_ = localizer
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exact := "admin:finance:report_100"
	exec(`INSERT INTO t_action (id,action_group,code,word,config) VALUES
 (101,$1,'GROUPRD1','admin:read:group:record','{"bindings":[],"ui":[],"endpoints":[]}'),(102,$1,'GROUPWR1','admin:update:group:record','{"bindings":[],"ui":[],"endpoints":[]}'),
 (103,$2,'GROUPSM1','admin:read:group:similar','{"bindings":[],"ui":[],"endpoints":[]}'),(104,'admin:group:other','GROUPUN1','admin:read:group:other','{"bindings":[],"ui":[],"endpoints":[]}')`, exact, exact+"_extra")
	exec(`INSERT INTO t_role (id,name,word,action) VALUES
		(101,'Group editor','group.editor',' GROUPWR1, GROUPRD1, GROUPRD1, MISSING1 '),
		(102,'Group reader','group.reader','GROUPRD1'),
		(103,'Group similar','group.similar','GROUPSM1'),
		(104,'Group other','group.other','GROUPUN1'),
		(105,'Group orphan','group.orphan','MISSING1'),
		(106,'Group empty','group.empty','')`)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false, localizer)
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
	localizer := testLocalization(t, store)
	_ = localizer
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	finance, operations, audit := "admin:finance:report_100", "admin:operations:record", "admin:audit:record"
	exec(`INSERT INTO t_action(id,action_group,code,word,config) VALUES
 (101,$1,'ALLFINR1','admin:read:all:finance','{"bindings":[],"ui":[],"endpoints":[]}'),(102,$1,'ALLFINW1','admin:update:all:finance','{"bindings":[],"ui":[],"endpoints":[]}'),
 (103,$2,'ALLOPSR1','admin:read:all:operations','{"bindings":[],"ui":[],"endpoints":[]}'),(104,$3,'ALLAUDR1','admin:read:all:audit','{"bindings":[],"ui":[],"endpoints":[]}'),
 (105,'admin:extra:record','ALLEXT01','admin:read:all:extra','{"bindings":[],"ui":[],"endpoints":[]}'),(106,'admin:operations:extra','ALLSPA01','admin:read:all:spaced','{"bindings":[],"ui":[],"endpoints":[]}'),
 (107,'admin:finance:record','ALLPAR01','admin:read:all:partial','{"bindings":[],"ui":[],"endpoints":[]}')`, finance, operations, audit)
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
	actions := action.New(store, limits, false, localizer)
	roles := role.New(store, limits, actions, false)
	size, one, two, three := int32(100), int32(1), int32(2), int32(3)
	empty, extra := "", "admin:extra:record"
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
		{"similar group remains independent", role.ListRolesInput{GroupsAll: []string{"admin:finance:record", operations, audit}}, 1, []int64{110}},
		{"empty group does not clear filter", role.ListRolesInput{GroupsAll: []string{""}}, 0, []int64{}},
		{"whitespace group does not clear filter", role.ListRolesInput{GroupsAll: []string{" "}}, 0, []int64{}},
		{"different group keys remain independent", role.ListRolesInput{GroupsAll: []string{finance, "admin:operations:extra", audit}}, 1, []int64{109}},
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
	localizer := testLocalization(t, store)
	_ = localizer
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
	roles := role.New(store, limits, action.New(store, limits, false, localizer), false)
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
	localizer := testLocalization(t, store)
	_ = localizer
	ctx := t.Context()
	_, err := store.DB.ExecContext(ctx, `INSERT INTO t_action(id,word,code,action_group,config) VALUES
 (101,'admin:read:qtest:qmatch-first','QKEY0001','admin:keyword:record','{"bindings":[],"ui":[],"endpoints":[]}'),
 (102,'admin:read:qtest:qmatch-other','QKEY0002','admin:keyword:record','{"bindings":[],"ui":[],"endpoints":[]}'),
 (103,'admin:read:qtest:third','QMATCH03','admin:other:record','{"bindings":[],"ui":[],"endpoints":[]}'),
 (104,'admin:read:qtest:qmatch-both','QMATCH04','admin:keyword:record','{"bindings":[],"ui":[],"endpoints":[]}'),
 (105,'admin:read:qtest:literal_100','QKEY0005','admin:keyword:record','{"bindings":[],"ui":[],"endpoints":[]}'),
 (106,'admin:read:qtest:literalx100','QKEY0006','admin:keyword:record','{"bindings":[],"ui":[],"endpoints":[]}')`)
	if err != nil {
		t.Fatal(err)
	}
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false, localizer)
	size, one, two, five := int32(100), int32(1), int32(2), int32(5)
	group := "admin:keyword:record"
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
		{"literal wildcards", action.ListActionsInput{Q: "literal_100"}, 1, []int64{105}},
		{"blank", action.ListActionsInput{Q: " "}, 6, []int64{106, 105, 104, 103, 102, 101}},
		{"missing", action.ListActionsInput{Q: "missing"}, 0, []int64{}},
		{"second page", action.ListActionsInput{Q: "qmatch", Page: &two, PageSize: &one}, 4, []int64{103}},
		{"past last page", action.ListActionsInput{Q: "qmatch", Page: &five, PageSize: &one}, 4, []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			input.Word = ":qtest:"
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
	localizer := testLocalization(t, store)
	_ = localizer
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
	actions := action.New(store, limits, false, localizer)
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

func TestActionLocalizedCatalogPostgres(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, false, localizer)
	if err != nil {
		t.Fatal(err)
	}
	if err := dictionaries.LoadLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(localizer.Reset)
	actions := action.New(store, limits, false, localizer)
	english := i18n.WithLocale(t.Context(), "en-US")
	chinese := i18n.WithLocale(t.Context(), "zh-CN")
	group := "admin:system:msg"
	size := int32(100)
	list := func(ctx context.Context, input action.ListActionsInput, total int64) *action.ListActionsResult {
		t.Helper()
		if input.PageSize == nil {
			input.PageSize = &size
		}
		input.Scope = "admin"
		input.GroupExact = &group
		value, err := actions.List(ctx, input)
		if err != nil || value.Total != total {
			t.Fatalf("localized list %+v: %+v %v", input, value, err)
		}
		return value
	}
	value := list(english, action.ListActionsInput{Q: "View message"}, 1)
	if len(value.Items) != 1 || value.Items[0].Name != "View message" || value.Items[0].Word != "admin:read:system:msg" {
		t.Fatalf("localized name: %+v", value)
	}
	readAction := value.Items[0]
	list(english, action.ListActionsInput{Q: readAction.Word}, 1)
	list(english, action.ListActionsInput{Q: readAction.Code}, 1)
	list(english, action.ListActionsInput{Name: "View message"}, 1)
	list(chinese, action.ListActionsInput{Q: "查看站内信"}, 1)
	list(chinese, action.ListActionsInput{Q: "View message"}, 0)
	list(chinese, action.ListActionsInput{Group: localizer.Current().Labels("zh-CN", "groups")[group]}, 3)
	groups, err := actions.ListGroups(chinese, localizer.Current().Labels("zh-CN", "groups")[group])
	if err != nil || len(groups.Items) != 1 || groups.Items[0] != group {
		t.Fatalf("localized group: %+v %v", groups, err)
	}
	one, two := int32(1), int32(2)
	value = list(english, action.ListActionsInput{Q: "message", Page: &two, PageSize: &one, Fields: report.Fields{"name"}}, 3)
	if len(value.Items) != 1 || value.Items[0].Name != "Send message" {
		t.Fatalf("localized pagination: %+v", value)
	}
	current := localizer.Current()
	current.Messages["en-US"].Frontend["actions"].(map[string]any)[readAction.Word] = "Inspect inbox 100%_!"
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(encoded)
	var dictionaryID int64
	if err := store.DB.QueryRowContext(t.Context(), `SELECT id FROM t_dictionary WHERE dictionary_key='I18N_CATALOG'`).Scan(&dictionaryID); err != nil {
		t.Fatal(err)
	}
	if _, err := dictionaries.Update(t.Context(), dictionaryID, dictionary.UpdateDictionaryInput{Value: &raw}); err != nil {
		t.Fatal(err)
	}
	// The committed dictionary update changes both the search predicate and labels without a build/restart.
	value = list(english, action.ListActionsInput{Q: "100%_!"}, 1)
	if value.Items[0].Name != "Inspect inbox 100%_!" {
		t.Fatal(value.Items[0].Name)
	}
	list(english, action.ListActionsInput{Q: "View message"}, 0)
	detail, err := actions.Get(english, readAction.ID)
	if err != nil || detail.Name != "Inspect inbox 100%_!" {
		t.Fatalf("detail after update: %+v %v", detail, err)
	}
	summary, err := actions.LookupSummaries(english, store.DB, []string{readAction.Code})
	if err != nil || summary[readAction.Code].Name != "Inspect inbox 100%_!" {
		t.Fatalf("summary after update: %+v %v", summary, err)
	}
}

func TestActionBindingNameFallbackPostgres(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	limits, _ := pagination.New(100, 100)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, false, localizer)
	if err != nil {
		t.Fatal(err)
	}
	actions := action.New(store, limits, false, localizer)
	actions.SetDefinitionCatalog(dictionaries)
	ctx := i18n.WithLocale(t.Context(), "en-US")
	value, err := actions.Create(ctx, action.CreateActionInput{
		Word:   "admin:read:custom:users",
		Config: action.Config{ResourceKey: "admin:system:user", Operation: "update", UI: []action.UIBinding{}, Endpoints: []action.Endpoint{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog := localizer.Current()
	want := catalog.Labels("en-US", "operations")["update"] + " " + catalog.Labels("en-US", "resources")["admin:system:user"]
	if value.Name != want {
		t.Fatalf("created name=%q want=%q", value.Name, want)
	}
	for _, input := range []action.ListActionsInput{
		{Code: value.Code, Q: want, Fields: report.Fields{"name"}},
		{Code: value.Code, Name: want, Fields: report.Fields{"name"}},
	} {
		list, err := actions.List(ctx, input)
		if err != nil || list.Total != 1 || len(list.Items) != 1 || list.Items[0].Name != want {
			t.Fatalf("binding name filter %+v: %+v %v", input, list, err)
		}
	}
	summaries, err := actions.LookupSummaries(ctx, store.DB, []string{value.Code})
	if err != nil || summaries[value.Code].Name != want {
		t.Fatalf("summary=%+v err=%v", summaries, err)
	}
}
