package tests

import (
	"context"
 "crypto/hmac"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/mailtemplate"
)

func newTemplateEmailCodes(store *db.Store, sender auth.EmailSender, cfg auth.EmailConfig, key string) (*auth.EmailCodes, error) {
	cfg.Templates = auth.EmailTemplateIDs{EmailLinked: "auth.email_linked", SignInCode: "auth.sign_in_code", EmailBindingCode: "auth.email_binding_code", SensitiveOperationCode: "auth.sensitive_operation_code", AuthenticatorResetCode: "auth.totp_reset_code", ConfirmEmailUnlinking: "auth.confirm_email_unlinking", EmailUnlinked: "auth.email_unlinked", AuthenticatorLinked: "auth.totp_linked", AuthenticatorRelinked: "auth.totp_linked", SensitiveOperationNotification: "auth.sensitive_operation_notification"}
	cfg.AuditURL = "https://example.test/audit/operation"
	codes, err := auth.NewEmailCodes(store, sender, cfg, key)
	if err == nil {
		limits, _ := pagination.New(100, 100)
		codes.SetEmailTemplates(mailtemplate.New(store, limits))
	}
	return codes, err
}

func TestPublishedEmailDeliveryUsesLatestSnapshot(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	module := mailtemplate.New(store, limits)
	ctx := t.Context()
	var id int64
	if err := store.DB.QueryRowContext(ctx, `SELECT id FROM t_mail_template WHERE template_id='auth.sign_in_code'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"account": "<owner>", "code": "765432", "expires_at": "2030-01-01T00:00:00Z"}
	subject, plain, html, err := module.RenderPublished(ctx, "auth.sign_in_code", "zh-CN", values)
	if err != nil || !strings.Contains(plain, "765432") || !strings.Contains(html, "&lt;o***") || strings.Contains(plain, "482916") {
		t.Fatal("initial rendering", err)
	}
	current, err := module.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for locale, translation := range current.Content {
		translation.Subject = "Updated publication"
		current.Content[locale] = translation
	}
	draft, err := module.Update(ctx, id, mailtemplate.UpdateInput{Revision: current.Revision, Content: &current.Content})
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, err := module.RenderPublished(ctx, "auth.sign_in_code", "zh-CN", values)
	if err != nil || got != subject {
		t.Fatal("draft affected delivery", err)
	}
	if _, err = module.Publish(ctx, id, mailtemplate.PublishInput{Revision: draft.Revision}); err != nil {
		t.Fatal(err)
	}
	got, _, _, err = module.RenderPublished(ctx, "auth.sign_in_code", "zh-CN", values)
	if err != nil || !strings.Contains(got, "Updated publication") {
		t.Fatal("publication not live", err)
	}
	if _, _, _, err = module.RenderPublished(ctx, "auth.sign_in_code", "zh-CN", map[string]string{"account": "owner"}); err == nil {
		t.Fatal("preview values used")
	}
	if err = module.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = module.RenderPublished(ctx, "auth.sign_in_code", "zh-CN", values); err == nil {
		t.Fatal("deleted template rendered")
	}
}

// Exercise every seeded template through the real outbox/River worker. The sink
// rejects incomplete MIME inputs and verifies delivery is finalized only afterward.
type templateDeliverySink struct {
 emailSink
 language string
}

func (s *templateDeliverySink) SendHTML(ctx context.Context, recipient, subject, plain, html string) error {
	if !strings.Contains(html, `<div lang="`+s.language+`">`) || strings.Contains(html, `<div lang="`+map[string]string{"en-US": "zh-CN", "zh-CN": "en-US"}[s.language]+`">`) || subject == "" || plain == "" || !strings.Contains(html, "<div") || strings.Contains(plain, "example-user") || strings.Contains(plain, "482916") {
		return mailtemplate.ErrInvalid
	}
	return s.emailSink.SendHTML(ctx, recipient, subject, plain, html)
}

func TestAllAuthEmailTemplatesThroughRiver(t *testing.T) {
	for _, metadata := range []string{`{}`, `{"language":null}`, `{"language":""}`, `{"language":"zh-CN"}`, `{"language":"en-US"}`, `{"language":"fr-FR"}`} {
		t.Run(metadata, func(t *testing.T) { testAuthEmailLanguageThroughRiver(t, metadata) })
	}
}

func testAuthEmailLanguageThroughRiver(t *testing.T, metadata string) {
	store := permissionDatabase(t)
	ctx := t.Context()
	language := "zh-CN"
	if strings.Contains(metadata, "en-US") { language = "en-US" }
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET metadata=$1::jsonb WHERE id=1`, metadata); err != nil { t.Fatal(err) }
	sink := &templateDeliverySink{language: language}
	codes, err := newTemplateEmailCodes(store, sink, auth.EmailConfig{UnlinkURL: "https://example.test/auth/unbind-email", CodeTTL: time.Minute, ResendInterval: time.Minute, MaxAttempts: 3, SendLimit: 1, SensitiveLifetime: 10 * time.Minute}, strings.Repeat("k", 64))
	if err != nil {
		t.Fatal(err)
	}
	purposes := []string{"login", "bind", "sensitive", "totp_reset", "unlink", "unlink_notice", "email_linked", "totp_bound", "totp_rebound", "sensitive_notice"}
	for _, purpose := range purposes {
		_, err = store.DB.ExecContext(ctx, `INSERT INTO t_auth_email_code(verification_id,client_ip,purpose,identifier,user_id,recipient,credential_version,code_hash,expired_at,delivery_attempts,notice_detail) VALUES($1,'',$2,'',1,'sink@example.test',1,'',$3,0,'3 sensitive operations within 5m')`, "template-"+purpose, purpose, time.Now().Add(time.Minute).UnixMilli())
		if err != nil {
			t.Fatal(err)
		}
	}
	sealFixtureEmails(t,store)
	stop, err := codes.StartWorkers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		var count int
		if err = store.DB.QueryRowContext(ctx, `SELECT count(*) FROM t_auth_email_code WHERE verification_id LIKE 'template-%' AND delivered`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == len(purposes) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("not all template purposes delivered")
}

// Sign synthetic renderer fixtures using their known test key. Production writes
// seal in the insertion transaction; this helper is never available to the app.
func sealFixtureEmails(t *testing.T,store *db.Store) {
 t.Helper()
 rows,err:=store.DB.QueryContext(t.Context(),`SELECT t.verification_id,COALESCE(t.user_id,0),t.recipient,t.purpose,t.created_at,t.expired_at,t.notice_detail,u.username,COALESCE(u.metadata->>'language','') FROM t_auth_email_code t JOIN t_user u ON u.id=t.user_id WHERE t.verification_id LIKE 'template-%'`)
 if err!=nil{t.Fatal(err)}
 type fixture struct{id,account,language,signature string}
 var fixtures []fixture
 for rows.Next(){
  var f fixture;var uid,created,expires int64;var recipient,purpose,detail string
  if err=rows.Scan(&f.id,&uid,&recipient,&purpose,&created,&expires,&detail,&f.account,&f.language);err!=nil{t.Fatal(err)}
  raw,_:=json.Marshal([]any{f.id,uid,recipient,purpose,created,expires,detail,f.account,f.language})
  mac:=hmac.New(sha256.New,[]byte(strings.Repeat("k",64)))
  mac.Write([]byte("auth-email\x00delivery-v1\x00"+string(raw)))
  f.signature=hex.EncodeToString(mac.Sum(nil));fixtures=append(fixtures,f)
 }
 err=rows.Err();rows.Close();if err!=nil{t.Fatal(err)}
 for _,f:=range fixtures{
  if _,err=store.DB.ExecContext(t.Context(),`UPDATE t_auth_email_code SET delivery_account=$2,delivery_language=$3,delivery_mac=$4 WHERE verification_id=$1`,f.id,f.account,f.language,f.signature);err!=nil{t.Fatal(err)}
 }
}
