package tests

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/msg"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
	"{{ .Computed.module_name_final }}/internal/modules/usergroup"
	"{{ .Computed.module_name_final }}/internal/modules/whitelist"
)

func TestManagementSummaryPostgres(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	// permissionDatabase gives this test its own schema, never the application's tables.
	exec(`TRUNCATE t_msg, t_whitelist, t_dictionary, t_user, t_user_group, t_role, t_action RESTART IDENTITY CASCADE`)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false)
	roles := role.New(store, limits, actions, false)
	users := user.New(store, limits, nil, nil, actions, roles, auth.Switches{})
	groups := usergroup.New(store, limits, actions, users)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, false)
	if err != nil {
		t.Fatal(err)
	}
	whitelists := whitelist.New(store, limits)
	messages := msg.New(store, limits)
	operations := []func(context.Context, ...string) (*report.ManagementSummary, error){actions.Summary, roles.Summary, groups.Summary, dictionaries.Summary, whitelists.Summary, messages.SentSummary}
 // Selective aggregates must also return one row on an empty database.
 for _, operation := range operations {
  value, err := operation(ctx, "total")
  if err != nil || value.Total != 0 { t.Fatalf("selected total: %#v %v", value, err) }
  if _, err := operation(ctx, "counts"); err != nil { t.Fatal(err) }
 }
 for _, operation := range []func(context.Context, ...string) (*report.ManagementSummary, error){actions.Summary, roles.Summary, groups.Summary} {
  value, err := operation(ctx, "groups")
  if err != nil || len(value.Groups) != 0 { t.Fatalf("empty selected groups: %#v %v", value, err) }
 }
 selectedUsers := user.New(store, limits, nil, nil, actions, roles, auth.Switches{})
 for _, fields := range []report.Fields{ {"total"}, {"counts"}, {"trend"}, {"total", "counts"}, {"total", "trend"}} {
  value, err := selectedUsers.Summary(ctx, user.SummaryInput{Fields:fields})
  if err != nil || value.Total != 0 { t.Fatalf("selected user %v: %#v %v", fields, value, err) }
 }
	for _, operation := range operations {
		value, err := operation(ctx)
		if err != nil || value.Total != 0 || value.Groups == nil || value.Trend == nil {
			t.Fatalf("empty summary %#v %v", value, err)
		}
		for k, n := range value.Counts {
			if n != 0 {
				t.Fatalf("empty %s=%d", k, n)
			}
		}
	}
	initial, err := users.Summary(ctx, user.SummaryInput{Days: 14, TimeZone: "Asia/Shanghai"})
	if err != nil || initial.Total != 0 || len(initial.Trend) != 14 {
		t.Fatalf("empty users %#v %v", initial, err)
	}
	first := initial.Trend[0]
	now := time.Now().UnixMilli()
	dates := []int64{first.From - 1, first.From, first.To - 1, first.To, initial.Trend[13].From}
	states := []int{0, 1, 2, 2, 2}
	metadata := []string{`{}`, `{}`, fmt.Sprintf(`{"lock_expired_at":%d}`, now-3600000), fmt.Sprintf(`{"lock_expired_at":%d}`, now+3600000), `{"lock_expired_at":0}`}
	for i, at := range dates {
		exec(`INSERT INTO t_user (id,username,code,password,status,metadata,created_at,updated_at) VALUES ($1,$2,$3,'hash',$4,$5,$6,42)`, i+1, fmt.Sprintf("summary%d", i), fmt.Sprintf("SUMUSR%02d", i), states[i], metadata[i], at)
	}
	exec(`INSERT INTO t_action (id,name,action_group,code,word,resource,menu,btn) VALUES (1,'Read','Group A','SUMACT01','summary.read','GET|/test','/test',''),(2,'Write','Group A','SUMACT02','summary.write','POST|/test','','test.write'),(3,'Other','Group B','SUMACT03','summary.empty','','','')`)
	exec(`INSERT INTO t_role (id,name,word,action) VALUES (1,'Reader','summary.reader','SUMACT01,SUMACT02'),(2,'Empty','summary.empty','MISSING1'),(3,'Duplicate','summary.duplicate','SUMACT01,SUMACT01')`)
	exec(`INSERT INTO t_user_group (id,name,word,action) VALUES (1,'First','summary.first','SUMACT01'),(2,'Second','summary.second','MISSING1'),(3,'Empty','summary.empty','')`)
	exec(`INSERT INTO t_user_user_group_relation (user_id,user_group_id) VALUES (1,1),(2,1),(1,2)`)
	exec(`INSERT INTO t_dictionary (dictionary_key,name,value,enabled) VALUES ('SUMMARY_ENABLED','Enabled','[]',true),('SUMMARY_DISABLED','Disabled','{}',false)`)
	exec(`INSERT INTO t_whitelist (category,resource) VALUES (0,'GET|/a'),(1,'GET|/b'),(1,'GET|/c')`)
	exec(`INSERT INTO t_msg (id,title,content,type,scope,published_at,expired_at) VALUES (1,'Literal 100%_','old','notice','all',$1,$2),(2,'Content search','Needle 100%_ here','system','targeted',$1,NULL),(3,'100xxxx','No match','notice','all',$3,NULL)`, now-7200000, now-3600000, now-1000)
	exec(`INSERT INTO t_msg_recipient (msg_id,user_id,read_at,deleted_at) VALUES (1,1,$1,$1),(2,1,NULL,NULL)`, now)
	check := func(value *report.ManagementSummary, err error, total int64, counts map[string]int64) {
		t.Helper()
		if err != nil || value.Total != total {
			t.Fatalf("summary %#v %v; total %d", value, err, total)
		}
		for key, n := range counts {
			if value.Counts[key] != n {
				t.Fatalf("%s=%d want %d (%#v)", key, value.Counts[key], n, value)
			}
		}
	}
	// Explicit read-only transaction guards against persisting expired user locks.
	err = store.Tx(ctx, func(tx context.Context) error {
		if _, err := store.SQL(tx).ExecContext(tx, `SET TRANSACTION READ ONLY`); err != nil {
			return err
		}
		value, err := users.Summary(tx, user.SummaryInput{Days: 14, TimeZone: "Asia/Shanghai"})
		check(value, err, 5, map[string]int64{"pending": 1, "active": 2, "locked": 2})
		if value.Trend[0].Count != 2 || value.Trend[1].Count != 1 || value.Trend[13].Count != 1 {
			t.Fatalf("calendar counts: %#v", value.Trend)
		}
		size := int32(100)
		rows, err := users.List(tx, user.ListUsersInput{CreatedFrom: &first.From, CreatedTo: &first.To, PageSize: &size})
		if err != nil || rows.Total != 2 || len(rows.Items) != 2 || rows.Items[0].ID != 3 || rows.Items[1].ID != 2 {
			t.Fatalf("exclusive range: %#v %v", rows, err)
		}
		if rows.Items[0].Status != user.StatusActive || rows.Items[0].UpdatedAt != 42 {
			t.Fatalf("effective user: %#v", rows.Items[0])
		}
		value, err = roles.Summary(tx)
		check(value, err, 3, map[string]int64{"with_permissions": 2, "without_permissions": 1})
		if len(value.Groups) != 1 || value.Groups[0].Count != 3 {
			t.Fatalf("role groups: %#v", value.Groups)
		}
		assigned, unassigned := true, false
		assignedRoles, err := roles.List(tx, role.ListRolesInput{HasPermissions: &assigned, PageSize: &size})
		if err != nil || assignedRoles.Total != 2 {
			t.Fatalf("assigned role filter: %#v %v", assignedRoles, err)
		}
		emptyRoles, err := roles.List(tx, role.ListRolesInput{HasPermissions: &unassigned, PageSize: &size})
		if err != nil || emptyRoles.Total != 1 || emptyRoles.Items[0].ID != 2 {
			t.Fatalf("orphan role filter: %#v %v", emptyRoles, err)
		}
		value, err = groups.Summary(tx)
		check(value, err, 3, map[string]int64{"memberships": 3, "unique_members": 2, "empty_groups": 1})
		if len(value.Groups) != 3 || value.Groups[0].ID != 1 || value.Groups[0].Count != 2 {
			t.Fatalf("group rank: %#v", value.Groups)
		}
		emptyGroups, err := groups.List(tx, usergroup.ListUserGroupsInput{HasMembers: &unassigned, PageSize: &size})
		if err != nil || emptyGroups.Total != 1 || emptyGroups.Items[0].ID != 3 {
			t.Fatalf("empty member filter: %#v %v", emptyGroups, err)
		}
		assignedGroups, err := groups.List(tx, usergroup.ListUserGroupsInput{HasPermissions: &assigned, HasMembers: &assigned, PageSize: &size})
		if err != nil || assignedGroups.Total != 1 || assignedGroups.Items[0].ID != 1 {
			t.Fatalf("group relation filters: %#v %v", assignedGroups, err)
		}
		value, err = actions.Summary(tx)
		check(value, err, 3, map[string]int64{"resource": 2, "menu": 1, "button": 1})
		if len(value.Groups) != 2 || value.Groups[1].Name != "Group B" || value.Groups[1].Count != 1 {
			t.Fatalf("ungrouped: %#v", value.Groups)
		}
		exact := "Group B"
		actionRows, err := actions.List(tx, action.ListActionsInput{GroupExact: &exact})
		if err != nil || actionRows.Total != 1 || actionRows.Items[0].ID != 3 {
			t.Fatalf("exact group: %#v %v", actionRows, err)
		}
		value, err = dictionaries.Summary(tx)
		check(value, err, 2, map[string]int64{"enabled": 1, "disabled": 1})
		value, err = whitelists.Summary(tx)
		check(value, err, 3, map[string]int64{"permission": 1, "jwt": 2})
		value, err = messages.SentSummary(tx)
		check(value, err, 3, map[string]int64{"notice": 2, "system": 1, "all": 2, "targeted": 1, "latest_published_at": now - 1000})
		sent, err := messages.ListSent(tx, msg.ListInput{Query: "100%_", PageSize: &size})
		if err != nil || sent.Total != 2 || len(sent.Items) != 2 {
			t.Fatalf("escaped search: %#v %v", sent, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var storedStatus int
	var updated int64
	if err := store.DB.QueryRowContext(ctx, `SELECT status,updated_at FROM t_user WHERE id=3`).Scan(&storedStatus, &updated); err != nil || storedStatus != 2 || updated != 42 {
		t.Fatalf("summary persisted unlock: %d %d %v", storedStatus, updated, err)
	}
	exec(`DELETE FROM t_msg WHERE id=3`)
	for i := 4; i <= 13; i++ {
		exec(`INSERT INTO t_user_group (id,name,word) VALUES ($1,$2,$3)`, i, fmt.Sprintf("Extra %d", i), fmt.Sprintf("extra.%d", i))
	}
	ranked, err := groups.Summary(ctx)
	if err != nil || ranked.Total != 13 || len(ranked.Groups) != 8 || ranked.Groups[0].ID != 1 || ranked.Groups[7].ID != 8 {
		t.Fatalf("top-eight group rank: %#v %v", ranked, err)
	}
	value, err := messages.SentSummary(ctx)
	check(value, err, 2, map[string]int64{"notice": 1, "system": 1, "latest_published_at": now - 7200000})
}

func TestManagementSummaryPermissionBoundary(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE t_user SET role_id=NULL,action='',login_count=1 WHERE id=2`)
	var identity authn.Identity
	if err := store.DB.QueryRowContext(ctx, `SELECT id,username,code,credential_version FROM t_user WHERE id=2`).Scan(&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions(auth.NewMemorySessionStore(), time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := authn.New("summary-test", strings.Repeat("x", 32), time.Hour, auth.NewIdentityValidator(store, sessions, auth.Switches{PasswordResetRequired: true}))
	if err != nil {
		t.Fatal(err)
	}
	session, err := sessions.Issue(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = session.SessionID
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	authentication := auth.New(store, manager, nil, sessions, nil, nil, nil, auth.Switches{PasswordResetRequired: true})
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false)
	roles := role.New(store, limits, actions, false)
	users := user.New(store, limits, nil, nil, actions, roles, auth.Switches{})
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Auth.Authorization.Enabled = true
	cfg.Idempotency.TTL = time.Minute
	handler, err := server.NewRouter(cfg, manager, idempotency.NewMemoryStore(), authentication, actions, roles, users, usergroup.New(store, limits, actions, users), dictionaries, whitelist.New(store, limits), msg.New(store, limits))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ path, read, write string }{
		{"/user/summary", "user.read", "user.create"}, {"/role/summary", "role.read", "role.update"}, {"/user-group/summary", "user.group.read", "user.group.update"}, {"/action/summary", "action.read", "action.delete"}, {"/dictionary/summary", "dictionary.read", "dictionary.update"}, {"/whitelist/summary", "whitelist.read", "whitelist.create"}, {"/msg/sent/summary", "msg.read", "msg.send"},
	}
	request := func(path string, authorized bool, want int) {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		if authorized {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d want %d: %s", path, w.Code, want, w.Body.String())
		}
	}
	for _, tc := range cases {
		request(tc.path, false, 401)
		exec(`UPDATE t_user SET action='' WHERE id=2`)
		request(tc.path, true, 403)
		exec(`UPDATE t_user SET action=(SELECT code FROM t_action WHERE word=$1) WHERE id=2`, tc.write)
		request(tc.path, true, 403)
		exec(`UPDATE t_user SET action=(SELECT code FROM t_action WHERE word=$1) WHERE id=2`, tc.read)
		request(tc.path, true, 200)
		for _, other := range cases {
			if other.path != tc.path {
				request(other.path, true, 403)
			}
		}
	}
	exec(`UPDATE t_user SET action=(SELECT code FROM t_action WHERE word='msg.delete') WHERE id=2`)
	request("/msg/sent/summary", true, 403)
}

func TestRecentLoggedInUsersSummaryPostgres(t *testing.T) {
	for _, zone := range []string{"Asia/Shanghai", "America/New_York"} {
		t.Run(zone, func(t *testing.T) {
			store := permissionDatabase(t)
			ctx := t.Context()
			exec := func(query string, args ...any) {
				t.Helper()
				if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
					t.Fatal(err)
				}
			}
			// This test owns an isolated schema created by permissionDatabase.
			exec(`TRUNCATE t_user CASCADE`)
			limits, _ := pagination.New(100, 100)
			users := user.New(store, limits, nil, nil, nil, nil, auth.Switches{})
			fortnight, err := users.Summary(ctx, user.SummaryInput{Days: 14, TimeZone: zone})
			if err != nil || fortnight.Counts["recent_logged_in_users"] != 0 {
				t.Fatalf("empty fortnight: %#v %v", fortnight, err)
			}
			month, err := users.Summary(ctx, user.SummaryInput{Days: 30, TimeZone: zone})
			if err != nil || month.Counts["recent_logged_in_users"] != 0 {
				t.Fatalf("empty month: %#v %v", month, err)
			}
			from14, from30 := fortnight.Trend[0].From, month.Trend[0].From
			lastCompleteLogin := time.Now().Add(-time.Minute).UnixMilli()
			fixtures := []struct {
				name    string
				last    any
				logins  int64
				created int64
			}{
				{"never", nil, 0, lastCompleteLogin},
				{"zero", int64(0), 1000, lastCompleteLogin},
				{"before_month", from30 - 1, 3, from30 - 1000},
				{"month_boundary", from30, 2, from30 - 1000},
				{"before_fortnight", from14 - 1, 1, from30 - 1000},
				{"fortnight_boundary", from14, 99, from30 - 1000},
				{"old_account_recent", lastCompleteLogin, 145, from30 - 86400000},
				{"password_reset", lastCompleteLogin, 0, from30 - 86400000},
				{"future", month.Trend[len(month.Trend)-1].To + 86400000, 500, lastCompleteLogin},
				{"future_today", month.Trend[len(month.Trend)-1].To - 1, 500, month.Trend[len(month.Trend)-1].To - 1},
			}
			for i, fixture := range fixtures {
				exec(`INSERT INTO t_user (id,username,code,password,status,last_logged_in_at,login_count,created_at,updated_at) VALUES ($1,$2,$3,'hash',1,$4,$5,$6,42)`, i+1, fixture.name, fmt.Sprintf("RECENT%02d", i), fixture.last, fixture.logins, fixture.created)
			}
			assertSummary := func(days int, want int64) *report.ManagementSummary {
				t.Helper()
				value, err := users.Summary(ctx, user.SummaryInput{Days: days, TimeZone: zone})
				if err != nil || value.Total != int64(len(fixtures)) || value.Counts["recent_logged_in_users"] != want {
					t.Fatalf("%s days=%d recent users=%#v want %d err=%v", zone, days, value, want, err)
				}
				if value.Trend[0].From != map[int]int64{14: from14, 30: from30}[days] {
					t.Fatal("summary calendar unexpectedly changed during test")
				}
				if value.Trend[len(value.Trend)-1].To != month.Trend[len(month.Trend)-1].To {
					t.Fatal("today's public boundary must remain complete and stable")
				}
				var dailyTotal, newUserTotal int64
				for _, day := range value.Trend {
					newUserTotal += day.Count
					if day.RecentLoggedInUsers < 0 {
						t.Fatal("negative login users")
					}
					dailyTotal += day.RecentLoggedInUsers
				}
				if newUserTotal != 3 {
					t.Fatalf("future timestamp inside today's public interval leaked into creation aggregate: %d", newUserTotal)
				}
				if dailyTotal != want {
					t.Fatalf("daily login distribution sums to %d; KPI %d", dailyTotal, want)
				}
				return value
			}
			// The lower boundary is included; the millisecond before it belongs only
			// to the longer range. Null/zero/future values and high login_count are inert.
			first14, first30 := assertSummary(14, 3), assertSummary(30, 5)
			if first14.Trend[0].RecentLoggedInUsers != 1 || first30.Trend[0].RecentLoggedInUsers != 1 || first30.Trend[15].RecentLoggedInUsers != 1 || first30.Trend[16].RecentLoggedInUsers != 1 {
				t.Fatalf("login day boundaries: %#v / %#v", first14.Trend, first30.Trend)
			}
			// A fresh login moves the same account from its previous last-login day;
			// it does not preserve a daily-active history or add a second account.
			exec(`UPDATE t_user SET last_logged_in_at=$1 WHERE username='fortnight_boundary'`, lastCompleteLogin)
			moved14, moved30 := assertSummary(14, 3), assertSummary(30, 5)
			if moved14.Trend[0].RecentLoggedInUsers != 0 || moved30.Trend[16].RecentLoggedInUsers != 0 {
				t.Fatal("previous last-login bucket retained a moved account")
			}
			// More completed logins for the same account do not increase unique users.
			exec(`UPDATE t_user SET login_count=login_count+100,last_logged_in_at=$1 WHERE username='old_account_recent'`, lastCompleteLogin+1)
			assertSummary(14, 3)
			assertSummary(30, 5)
			// Removing the timestamp makes the account ineligible even with a count.
			exec(`UPDATE t_user SET last_logged_in_at=NULL WHERE username='old_account_recent'`)
			assertSummary(14, 2)
			assertSummary(30, 4)
			var resetCount, resetLast int64
			if err := store.DB.QueryRowContext(ctx, `SELECT login_count,last_logged_in_at FROM t_user WHERE username='password_reset'`).Scan(&resetCount, &resetLast); err != nil || resetCount != 0 || resetLast != lastCompleteLogin {
				t.Fatalf("summary changed reset account: %d %d %v", resetCount, resetLast, err)
			}
		})
	}
}

func TestUserCalendarDayDrilldownPostgres(t *testing.T) {
	for _, tc := range []struct {
		zone  string
		year  int
		month time.Month
		day   int
		hours int64
	}{
		{"Asia/Shanghai", 2026, time.October, 12, 24},
		{"America/New_York", 2026, time.March, 8, 23},
		{"America/New_York", 2026, time.November, 1, 25},
	} {
		t.Run(fmt.Sprintf("%s-%02d-%02d", tc.zone, tc.month, tc.day), func(t *testing.T) {
			store := permissionDatabase(t)
			ctx := t.Context()
			exec := func(query string, args ...any) {
				t.Helper()
				if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
					t.Fatal(err)
				}
			}
			exec(`TRUNCATE t_user CASCADE`)
			location, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			midnight := time.Date(tc.year, tc.month, tc.day, 0, 0, 0, 0, location)
			from, to := midnight.UnixMilli(), midnight.AddDate(0, 0, 1).UnixMilli()
			if to-from != tc.hours*int64(time.Hour/time.Millisecond) {
				t.Fatal("calendar day duration")
			}
			timestamps := []int64{from - 1, from, to - 1000, to - 1, to, from - 2, from - 2}
			for i, at := range timestamps {
				var last any = at
				if i == 5 {
					last = nil
				}
				if i == 6 {
					last = int64(0)
				}
				status := user.StatusActive
				if i == 3 {
					status = user.StatusPending
				}
				exec(`INSERT INTO t_user (id,username,code,password,status,created_at,updated_at,last_logged_in_at) VALUES ($1,$2,$3,'hash',$4,$5,42,$6)`, i+1, fmt.Sprintf("calendar%d", i), fmt.Sprintf("CALD%04d", i), status, at, last)
			}
			limits, _ := pagination.New(100, 100)
			actions := action.New(store, limits, false)
			roles := role.New(store, limits, actions, false)
			users := user.New(store, limits, nil, nil, actions, roles, auth.Switches{})
			size := int32(2)
			// Both timestamp columns include 23:59:59.999 and exclude the next midnight.
			for _, kind := range []string{"created", "last-login"} {
				input := user.ListUsersInput{PageSize: &size}
				if kind == "created" {
					input.CreatedFrom = &from
					input.CreatedTo = &to
				} else {
					input.LastLoggedInFrom = &from
					input.LastLoggedInTo = &to
				}
				first, err := users.List(ctx, input)
				if err != nil || first.Total != 3 || len(first.Items) != 2 || first.Items[0].ID != 4 || first.Items[1].ID != 3 {
					t.Fatalf("%s first page: %#v %v", kind, first, err)
				}
				page := int32(2)
				input.Page = &page
				second, err := users.List(ctx, input)
				if err != nil || second.Total != 3 || len(second.Items) != 1 || second.Items[0].ID != 2 {
					t.Fatalf("%s next page: %#v %v", kind, second, err)
				}
			}
			for _, input := range []user.ListUsersInput{
				{LastLoggedInFrom: &from, PageSize: &size}, {LastLoggedInTo: &to, PageSize: &size},
			} {
				value, err := users.List(ctx, input)
				if err != nil || value.Total != 4 {
					t.Fatalf("single-sided login range must exclude null/zero: %#v %v", value, err)
				}
			}
			value, err := users.List(ctx, user.ListUsersInput{CreatedFrom: &from, CreatedTo: &to, LastLoggedInFrom: &from, LastLoggedInTo: &to, Statuses: []int16{user.StatusActive}, PageSize: &size})
			if err != nil || value.Total != 2 || len(value.Items) != 2 || value.Items[0].ID != 3 || value.Items[1].ID != 2 {
				t.Fatalf("combined ranges/status: %#v %v", value, err)
			}
		})
	}
}
