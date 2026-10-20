package tests

import (
	"database/sql"
	"fmt"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
)

func TestDeletedReferencesUpdateTimestampsAtomically(t *testing.T) {
	for _, target := range []string{"role", "user"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/rollback=%t", target, fail), func(t *testing.T) {
				store := permissionDatabase(t)
				ctx := t.Context()
				limits, _ := pagination.New(100, 100)
				exec := func(statement string) {
					t.Helper()
					if _, err := store.SQL(ctx).ExecContext(ctx, statement); err != nil {
						t.Fatal(err)
					}
				}
				exec(`INSERT INTO t_role(id,name,word) VALUES (900001,'Reference test','reference-test');
UPDATE t_user SET role_id=900001, created_at=1000, updated_at=2000 WHERE id=1;
INSERT INTO t_msg(id,created_at,updated_at,title,content,type,scope,sender_id)
VALUES (900001,1000,2000,'Reference test','Body','notice','all',1)`)
				if fail {
					exec(`CREATE FUNCTION reject_test_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected delete failure'; END; $$`)
					exec("CREATE TRIGGER reject_test_delete BEFORE DELETE ON t_" + target + " FOR EACH ROW EXECUTE FUNCTION reject_test_delete()")
				}
				var err error
				query := "SELECT role_id,created_at,updated_at FROM t_user WHERE id=1"
				parentID := int64(900001)
				if target == "role" {
					err = role.New(store, limits, nil).Delete(ctx, parentID)
				} else {
					parentID = 1
					query = "SELECT sender_id,created_at,updated_at FROM t_msg WHERE id=900001"
					err = user.New(store, limits, nil, nil, nil, nil).Delete(ctx, parentID)
				}
				if (err != nil) != fail {
					t.Fatalf("delete error=%v, failure expected=%t", err, fail)
				}
				var reference sql.NullInt64
				var createdAt, updatedAt int64
				if err := store.SQL(ctx).QueryRowContext(ctx, query).Scan(&reference, &createdAt, &updatedAt); err != nil {
					t.Fatal(err)
				}
				if createdAt != 1000 {
					t.Fatalf("created_at changed: %d", createdAt)
				}
				if fail {
					if !reference.Valid || reference.Int64 != parentID || updatedAt != 2000 {
						t.Fatalf("failed deletion changed reference: %v updated=%d", reference, updatedAt)
					}
				} else if reference.Valid || updatedAt <= 2000 {
					t.Fatalf("detachment did not update timestamp: %v updated=%d", reference, updatedAt)
				}
			})
		}
	}
}
