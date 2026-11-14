package tests

import "testing"

func TestEmailLinkedNoticeQueuePostgres(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	if _, err := store.DB.ExecContext(ctx, `INSERT INTO t_auth_email_code
        (verification_id,client_ip,purpose,identifier,user_id,recipient,credential_version,code_hash,expired_at,delivery_attempts)
        VALUES ('linked-notice','','email_linked','',1,'linked@example.test',1,'',9999999999999,0)`); err != nil {
		t.Fatal(err)
	}
	var queue string
	if err := store.DB.QueryRowContext(ctx, `SELECT queue FROM river_job WHERE kind='auth_email_deliver' AND args->>'ticket_id'='linked-notice'`).Scan(&queue); err != nil || queue != "auth_email_notice" {
		t.Fatal("binding notice must use the notice queue", queue, err)
	}
}
