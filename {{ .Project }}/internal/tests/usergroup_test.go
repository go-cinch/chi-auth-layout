package tests

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/server"

	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
	"{{ .Computed.module_name_final }}/internal/modules/usergroup"
)

func TestThousandMemberGroup(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	ctx := t.Context()
	for _, query := range []string{
		`INSERT INTO t_user_group (id,name,word) VALUES (101,'Thousand','thousand'),(102,'Empty','empty')`,
		`INSERT INTO t_user (id,username,code,password,status) SELECT i, 'member_'||i, 'M'||lpad(i::text,7,'0'), 'test',1 FROM generate_series(1001,2002) i`,
		`INSERT INTO t_user_user_group_relation (user_group_id,user_id) SELECT 101,i FROM generate_series(1001,2000) i`,
	} {
		if _, err := store.DB.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, localizer)
	roles := role.New(store, limits, actions)
	users := user.New(store, limits, nil, nil, actions, roles)
	groups := usergroup.New(store, limits, actions, users)
	size, page := int32(20), int32(2)
	result, err := groups.List(ctx, usergroup.ListUserGroupsInput{Word: "thousand", PageSize: &size, Fields: report.Fields{"actions", "member_count", "member_preview"}})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Items[0]
	if *group.MemberCount != 1000 || len(*group.MemberPreview) != 5 || (*group.MemberPreview)[0].ID != 1001 || (*group.MemberPreview)[4].ID != 1005 || group.Users != nil {
		t.Fatalf("%+v", group)
	}
	data, _ := json.Marshal(group)
	if strings.Contains(string(data), `"users"`) || len(data) > 2000 {
		t.Fatalf("unbounded response: %s", data)
	}
	empty, err := groups.Get(ctx, 102, "member_count", "member_preview")
	if err != nil || *empty.MemberCount != 0 || len(*empty.MemberPreview) != 0 {
		t.Fatalf("%+v %v", empty, err)
	}
	members, err := groups.ListMembers(ctx, 101, usergroup.ListMembersInput{Page: &page, PageSize: &size})
	if err != nil || members.Total != 1000 || len(members.Items) != 20 || members.Items[0].ID != 1021 {
		t.Fatalf("%+v %v", members, err)
	}
	matches, err := groups.ListMembers(ctx, 101, usergroup.ListMembersInput{Q: "MEMBER_1001", PageSize: &size})
	if err != nil || matches.Total != 1 || matches.Items[0].ID != 1001 {
		t.Fatalf("%+v %v", matches, err)
	}
	matches, err = groups.ListMembers(ctx, 101, usergroup.ListMembersInput{Q: "%", PageSize: &size})
	if err != nil || matches.Total != 0 {
		t.Fatalf("literal wildcard: %+v %v", matches, err)
	}
	id := int64(101)
	options, err := users.List(ctx, user.ListUsersInput{Q: "member_", ExcludeUserGroupID: &id, PageSize: &size})
	if err != nil || options.Total != 2 {
		t.Fatalf("available: %+v %v", options, err)
	}
	name := "Renamed"
	if _, err := groups.Update(ctx, 101, usergroup.UpdateUserGroupInput{Name: &name}, "member_count"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for i := int64(0); i < 2; i++ {
		wg.Add(1)
		go func(i int64) {
			defer wg.Done()
			add, remove := []int64{2001 + i, 2001 + i}, []int64{1001 + i}
			_, err := groups.Update(ctx, 101, usergroup.UpdateUserGroupInput{AddUserIDs: &add, RemoveUserIDs: &remove}, "member_count")
			failures <- err
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	updated, err := groups.Get(ctx, 101, "member_count", "member_preview")
	if err != nil || *updated.MemberCount != 1000 || (*updated.MemberPreview)[0].ID != 1003 {
		t.Fatalf("%+v %v", updated, err)
	}
	var count int
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_user_user_group_relation WHERE user_group_id=101 AND user_id IN (2001,2002)`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("concurrent membership: %d %v", count, err)
	}
	add, remove := []int64{999999}, []int64{1003}
	if _, err := groups.Update(ctx, 101, usergroup.UpdateUserGroupInput{AddUserIDs: &add, RemoveUserIDs: &remove}, "member_count"); err == nil {
		t.Fatal("missing user accepted")
	}
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_user_user_group_relation WHERE user_group_id=101 AND user_id=1003`).Scan(&count); err != nil || count != 1 {
		t.Fatal(fmt.Sprintf("rollback: %d %v", count, err))
	}
	legacy, err := groups.Get(ctx, 101)
	if err != nil || len(legacy.Users) != 1000 {
		t.Fatalf("legacy compatibility: %v", err)
	}
}

func TestGroupMemberPermissions(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	if _, err := store.DB.ExecContext(t.Context(), `UPDATE t_user SET role_id=NULL,action=(SELECT code FROM t_action WHERE word='admin:read:system:user-group'),login_count=1 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	var identity authn.Identity
	if err := store.DB.QueryRowContext(t.Context(), `SELECT id,username,code,credential_version FROM t_user WHERE id=2`).Scan(&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	memorySessions := auth.NewMemorySessionStore()
	t.Cleanup(func() { _ = memorySessions.Close() })
	sessions, err := auth.NewSessions(memorySessions, auth.SessionConfig{
		Issuer:           "test",
		Key:              "session-test-signing-key-0123456789abcdef",
		SessionLifetime:  time.Hour,
		RememberLifetime: 24 * time.Hour,
		RetryWindow:      10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := authn.New("test", strings.Repeat("x", 32), time.Hour, auth.NewIdentityValidator(store, sessions))
	if err != nil {
		t.Fatal(err)
	}
	session, err := sessions.Issue(t.Context(), identity, false)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = session.SessionID
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, localizer)
	roles := role.New(store, limits, actions)
	users := user.New(store, limits, nil, nil, actions, roles)
	groups := usergroup.New(store, limits, actions, users)
	authModule := auth.New(store, manager, nil, sessions, nil, nil, nil, localizer, nil)
	cfg := &config.Config{}
	cfg.Auth.Authorization.Enabled = true
	cfg.Idempotency.TTL = time.Minute
	handler, err := server.NewRouter(cfg, manager, idempotency.NewMemoryStore(), authModule, groups, users)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path  string
		authenticated bool
		status        int
	}{
		{"GET", "/user-group/1/member?p=1&s=20", true, 200},
		{"GET", "/user-group/1?f=member_count,member_preview", true, 200},
		{"PATCH", "/user-group/1?f=member_count", true, 403},
		{"GET", "/user?exclude_user_group_id=1&p=1&s=20", true, 403},
		{"GET", "/user-group/1/member", false, 401},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"remove_user_ids":[3]}`))
		if tc.authenticated {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
