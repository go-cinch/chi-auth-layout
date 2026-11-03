package tests

{{ if .Computed.enable_redis_final -}}
import (
    "context"
    "crypto/hmac"
    "crypto/sha1"
    "encoding/base32"
    "encoding/binary"
    "errors"
    "fmt"
    "net/http"
    "net/http/httptest"
    "strings"
    "sync/atomic"
    "testing"
    "time"

    "golang.org/x/crypto/bcrypt"
    "{{ .Computed.module_name_final }}/internal/common/auditctx"
    "{{ .Computed.module_name_final }}/internal/common/authn"
    "{{ .Computed.module_name_final }}/internal/common/config"
    "{{ .Computed.module_name_final }}/internal/common/pagination"
    "{{ .Computed.module_name_final }}/internal/common/server"
    "{{ .Computed.module_name_final }}/internal/modules/audit"
    "{{ .Computed.module_name_final }}/internal/modules/auth"
    "{{ .Computed.module_name_final }}/internal/modules/dictionary"
{{ if eq .Computed.http_router_final "gin" }}
    "github.com/gin-gonic/gin"
{{ else }}
    "github.com/go-chi/chi/v5"
{{ end }}
)

type mfaProbe struct { mutations atomic.Int32 }
func (*mfaProbe) Name() string {return "/mfa-probe"}
{{ if eq .Computed.http_router_final "gin" }}
func (m *mfaProbe) HTTP(r *gin.RouterGroup) {
    r.DELETE("/:id",func(c *gin.Context){m.mutations.Add(1);c.Status(http.StatusNoContent)})
}
{{ else }}
func (m *mfaProbe) HTTP() http.Handler {
    r:=chi.NewRouter();r.Delete("/{id}",func(w http.ResponseWriter,_ *http.Request){m.mutations.Add(1);w.WriteHeader(http.StatusNoContent)});return r
}
{{ end }}

func integrationTOTP(t *testing.T, secret string) string {
    t.Helper()
    // This fixture uses a one-second TOTP period. Wait for the observable next
    // step so successive operations use fresh authenticator codes without a replay bypass.
    timer:=time.NewTimer(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
    defer timer.Stop()
    select {case <-timer.C: case <-t.Context().Done(): t.Fatal(t.Context().Err())}
    key,err:=base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret);if err!=nil {t.Fatal(err)}
    var counter [8]byte;binary.BigEndian.PutUint64(counter[:],uint64(time.Now().Unix()))
    mac:=hmac.New(sha1.New,key);_,_=mac.Write(counter[:]);sum:=mac.Sum(nil);offset:=sum[len(sum)-1]&15
    return fmt.Sprintf("%06d",(binary.BigEndian.Uint32(sum[offset:offset+4])&0x7fffffff)%1000000)
}

func TestMFARealDatabaseRedisLifecycle(t *testing.T) {
    store:=permissionDatabase(t)
    redis:=sessionRedisClient(t)
    localizer:=testLocalization(t,store)
    limits,_:=pagination.New(100,100)
    dictionaries,err:=dictionary.New(store,dictionary.NewMemoryValueCache(),dictionary.CacheTTL,limits,localizer)
    if err!=nil {t.Fatal(err)}
    captcha,err:=auth.NewPointCaptcha(auth.NewMemoryPointCaptchaStore(),dictionaries,5,time.Minute,300,180,22,nil)
    if err!=nil {t.Fatal(err)}
    slider,err:=auth.NewSliderCaptcha(auth.NewMemoryPointCaptchaStore(),auth.SliderCaptchaConfig{TTL:time.Minute,MinimumDuration:100*time.Millisecond})
    if err!=nil {t.Fatal(err)}
    guard,err:=auth.NewPasswordChangeGuard(auth.NewMemoryPasswordFailureStore(),captcha,3,6,time.Minute)
    if err!=nil {t.Fatal(err)}
    key:=strings.Repeat("k",64)
    sessions,err:=auth.NewSessions(auth.NewRedisSessionStore(redis),auth.SessionConfig{Issuer:"mfa-test",Key:key,SessionLifetime:time.Hour,RememberLifetime:24*time.Hour,RetryWindow:time.Second})
    if err!=nil {t.Fatal(err)}
    manager,err:=authn.New("mfa-test",key,time.Hour,auth.NewIdentityValidator(store,sessions))
    if err!=nil {t.Fatal(err)}
    var issued []*auth.LoginResult
    var tickets []string
    // Register before the first mutation so assertion failures also clean Redis.
    t.Cleanup(func(){
        cleanupCtx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
        for _,result:=range issued {
            id,err:=manager.Parse(result.AccessToken);if err==nil {
                if err:=sessions.Revoke(cleanupCtx,id.SessionID);err!=nil {t.Error(err)}
                if err:=redis.Del(cleanupCtx,"mfa:enrollment:"+id.SessionID,"mfa:sensitive:"+id.SessionID,"mfa:enrollment-grant:"+id.SessionID,fmt.Sprintf("mfa:attempt:%d:%d",id.UserID,id.CredentialVersion)).Err();err!=nil {t.Error(err)}
            }
        }
        for _,ticket:=range tickets {if err:=redis.Del(cleanupCtx,"mfa:login:"+ticket).Err();err!=nil {t.Error(err)}}
    })
    // This lifecycle deliberately exercises many valid and invalid MFA flows;
    // dedicated rate-limit tests cover the bounded fixed-window policy.
    mfa,err:=auth.NewMFA(store,auth.NewRedisMFAStore(redis),auth.MFAConfig{Enabled: true, LoginTicketTTL: 5*time.Minute, TOTPPeriod:time.Second,EnrollmentTTL:5*time.Minute,SensitiveLifetime:90*time.Second,RateLimitWindow:5*time.Minute,RateLimitMaxAttempts:32,Issuer:"mfa-test",SigningKey:key})
    if err!=nil {t.Fatal(err)}
    m:=auth.New(store,manager,nil,sessions,captcha,slider,guard,localizer,nil,mfa)
    manager.SetHTTPSecurityPolicy(m.CheckHTTPSecurity)
    hash,err:=bcrypt.GenerateFromPassword([]byte("initial"),bcrypt.MinCost);if err!=nil {t.Fatal(err)}
    if _,err:=store.DB.ExecContext(t.Context(),`INSERT INTO t_user (id,username,code,password,status,role_id) VALUES (101,'mfa-user','MFATEST1',$1,1,1)`,string(hash));err!=nil {t.Fatal(err)}
    login:=func(password string)*auth.LoginResult {
        t.Helper();input,err:=sliderLoginInput(t.Context(),slider,"mfa-user",password,false);if err!=nil {t.Fatal(err)}
        result,err:=m.Login(t.Context(),input);if err!=nil {t.Fatal(err)};if result.AccessToken!="" {issued=append(issued,result)};if result.MFATicket!="" {tickets=append(tickets,result.MFATicket)};return result
    }
    authenticate:=func(token,path string)context.Context {
        t.Helper();method:="POST";if path=="/reset/pwd" {method="PATCH"};r:=httptest.NewRequest(method,path,nil);r.Header.Set("Authorization","Bearer "+token)
        request,err:=manager.AuthenticateRequest(r);if err!=nil {t.Fatal(err)};return request.Context()
    }
    first:=login("initial")
    if !first.PasswordResetRequired || !first.MFAEnrollmentRequired {t.Fatal("first login must require both MFA enrollment and password reset")}
    resetRequest:=httptest.NewRequest("PATCH","/reset/pwd",nil)
    resetRequest.Header.Set("Authorization","Bearer "+first.AccessToken)
    authenticatedReset,err:=manager.AuthenticateRequest(resetRequest);if err!=nil {t.Fatal(err)}
    if err:=manager.CheckHTTPSecurity(authenticatedReset);!errors.Is(err,auth.ErrMFAEnrollmentRequired) {t.Fatal("password reset bypassed MFA enrollment",err)}
    restrictedCtx:=authenticate(first.AccessToken,"/mfa/setup")
    if _,err:=m.ResetPassword(restrictedCtx,auth.PasswordResetInput{NewPassword:"changed"});!errors.Is(err,auth.ErrMFAEnrollmentRequired) {t.Fatal("direct password reset bypassed MFA enrollment",err)}
    probe:=&mfaProbe{}
    router,err:=server.NewRouter(&config.Config{},manager,nil,m,probe)
    if err!=nil {t.Fatal(err)}
    deletion:=func(token string)int {
        t.Helper();r:=httptest.NewRequest("DELETE","/mfa-probe/1",nil);r.Header.Set("Authorization","Bearer "+token)
        response:=httptest.NewRecorder();router.ServeHTTP(response,r);return response.Code
    }
    if deletion(first.AccessToken)!=403 || probe.mutations.Load()!=0 {t.Fatal("mandatory binding bypassed")}
    status,err:=m.MFAStatus(restrictedCtx);if err!=nil || status.PasswordRequired || !status.EnrollmentRequired {t.Fatal("login did not authorize direct enrollment",err)}
    refreshedFirst,err:=m.Refresh(t.Context(),first.RefreshToken);if err!=nil {t.Fatal(err)}
    issued=append(issued,refreshedFirst)
    if !refreshedFirst.MFAEnrollmentRequired || !refreshedFirst.PasswordResetRequired {t.Fatal("refresh lost onboarding restrictions")}
    restrictedCtx=authenticate(refreshedFirst.AccessToken,"/mfa/setup")
    resumed:=login("initial")
    resumedStatus,err:=m.MFAStatus(authenticate(resumed.AccessToken,"/mfa/setup"))
    if err!=nil || resumedStatus.PasswordRequired {t.Fatal("password login did not authorize mandatory enrollment",err)}
    setup,err:=m.BeginMFAEnrollment(restrictedCtx,"");if err!=nil {t.Fatal(err)}
    enrolled,err:=m.ConfirmMFAEnrollment(restrictedCtx,integrationTOTP(t,setup.Secret));if err!=nil {t.Fatal(err)}
    issued=append(issued,enrolled.LoginResult)
    if enrolled.MFAEnrollmentRequired || !enrolled.PasswordResetRequired {t.Fatal("binding must preserve password reset requirement")}
    if deletion(first.AccessToken)!=401 {t.Fatal("binding did not invalidate prior session")}
    // Already-bound first-login accounts must verify MFA before password reset too.
    pendingReset:=login("initial")
    if !pendingReset.MFARequired || pendingReset.AccessToken!="" {t.Fatal("bound first login bypassed MFA")}
    verifiedReset,err:=m.CompleteMFALogin(t.Context(),pendingReset.MFATicket,integrationTOTP(t,setup.Secret));if err!=nil {t.Fatal(err)}
    issued=append(issued,verifiedReset)
    if !verifiedReset.PasswordResetRequired || verifiedReset.MFAEnrollmentRequired {t.Fatal("wrong state after login verification")}
    reset,err:=m.ResetPassword(authenticate(verifiedReset.AccessToken,"/reset/pwd"),auth.PasswordResetInput{NewPassword:"changed"})
    if err!=nil || reset.MFAEnrollmentRequired || reset.PasswordResetRequired {t.Fatal("password reset did not finish onboarding",err)}
    issued=append(issued,reset)
    var stored string
    if err:=store.DB.QueryRowContext(t.Context(),`SELECT secret_ciphertext FROM t_user_totp WHERE user_id=101`).Scan(&stored);err!=nil {t.Fatal(err)}
    if strings.Contains(stored,setup.Secret) {t.Fatal("plaintext TOTP secret stored")}
    if deletion(enrolled.AccessToken)!=401 {t.Fatal("password reset did not invalidate prior session")}
    if deletion(reset.AccessToken)!=403 {t.Fatal("enrollment created sensitive grant")}
    pending:=login("changed")
    if !pending.MFARequired || pending.AccessToken!="" || pending.RefreshToken!="" {t.Fatal("password-only login returned session")}
    loggedIn,err:=m.CompleteMFALogin(t.Context(),pending.MFATicket,integrationTOTP(t,setup.Secret));if err!=nil {t.Fatal(err)}
    issued=append(issued,loggedIn)
    if deletion(loggedIn.AccessToken)!=403 {t.Fatal("login granted sensitive access")}
    if _,err:=m.CompleteMFALogin(t.Context(),pending.MFATicket,"123456");!errors.Is(err,auth.ErrMFAInvalid) {t.Fatal("login ticket replay accepted",err)}
    testCode:=func(code string)*httptest.ResponseRecorder {
        r:=httptest.NewRequest("POST","/mfa/test",strings.NewReader(`{"verification_code":"`+code+`"}`))
        r.Header.Set("Authorization","Bearer "+loggedIn.AccessToken)
        w:=httptest.NewRecorder();router.ServeHTTP(w,r);return w
    }
    testIdentity,err:=manager.Parse(loggedIn.AccessToken);if err!=nil {t.Fatal(err)}
    attemptKey:=fmt.Sprintf("mfa:attempt:%d:%d",testIdentity.UserID,testIdentity.CredentialVersion)
    testAttemptsBefore,err:=redis.Eval(t.Context(),`return tonumber(redis.call('GET', KEYS[1]) or '0')`,[]string{attemptKey}).Int64();if err!=nil {t.Fatal(err)}
    testedCode:=integrationTOTP(t,setup.Secret)
    if w:=testCode(testedCode);w.Code!=200 || !strings.Contains(w.Body.String(),`"verified":true`) || w.Header().Get(authn.MFARequiredHeader)!="" {t.Fatal("authenticator test failed",w.Code)}
    if w:=testCode(testedCode);w.Code!=400 || w.Header().Get(authn.MFARequiredHeader)!="" {t.Fatal("test code replay accepted or requested sensitive verification",w.Code)}
    testAttemptsAfter,err:=redis.Eval(t.Context(),`return tonumber(redis.call('GET', KEYS[1]) or '0')`,[]string{attemptKey}).Int64()
    if err!=nil || testAttemptsAfter!=testAttemptsBefore+2 {t.Fatal("successful and replayed tests did not share attempt budget",err)}
    if deletion(loggedIn.AccessToken)!=403 {t.Fatal("authenticator test created sensitive grant")}
    testLogin:=login("changed")
    if _,err:=m.CompleteMFALogin(t.Context(),testLogin.MFATicket,testedCode);!errors.Is(err,auth.ErrMFAInvalid) {t.Fatal("tested code reused for login",err)}
    if _,err:=m.VerifySensitiveMFA(authenticate(loggedIn.AccessToken,"/mfa/test"),testedCode);!errors.Is(err,auth.ErrMFAInvalid) {t.Fatal("tested code reused for sensitive verification",err)}
    inlineDelete:=func(code string)*httptest.ResponseRecorder {
        r:=httptest.NewRequest("DELETE","/mfa-probe/1",nil)
        r.Header.Set("Authorization","Bearer "+loggedIn.AccessToken)
        r.Header.Set(authn.MFACodeHeader,code)
        w:=httptest.NewRecorder();router.ServeHTTP(w,r);return w
    }
    beforeMutations:=probe.mutations.Load()
    for _,code:=range []string{"","invalid-recovery-code",enrolled.RecoveryCodes[4]} {
        w:=inlineDelete(code)
        if (w.Code!=403 && w.Code!=400) || w.Header().Get(authn.MFARequiredHeader)!="true" || probe.mutations.Load()!=beforeMutations {t.Fatal("inline MFA gate bypass",w.Code)}
    }
    if w:=inlineDelete(integrationTOTP(t,setup.Secret));w.Code!=204 || w.Header().Get(authn.MFARequiredHeader)!="" || probe.mutations.Load()!=beforeMutations+1 {t.Fatal("same-endpoint MFA did not execute mutation",w.Code)}
    ctx:=authenticate(loggedIn.AccessToken,"/mfa")
    // The same TOTP step submitted concurrently must produce exactly one grant.
    concurrentCode:=integrationTOTP(t,setup.Secret)
    var winners atomic.Int32
    results:=make(chan error,2)
    for range 2 {go func(){_,err:=m.VerifySensitiveMFA(ctx,concurrentCode);if err==nil {winners.Add(1)};results<-err}()}
    for range 2 {err:=<-results;if err!=nil && !errors.Is(err,auth.ErrMFAInvalid) {t.Fatal(err)}}
    if winners.Load()!=1 {t.Fatal("non-atomic TOTP consumption",winners.Load())}
    if deletion(loggedIn.AccessToken)!=204 || deletion(loggedIn.AccessToken)!=204 {t.Fatal("valid sensitive window rejected")}
    if deletion(reset.AccessToken)!=403 {t.Fatal("sensitive grant leaked to another session")}
    beforeRefreshIdentity,err:=manager.Parse(loggedIn.AccessToken);if err!=nil {t.Fatal(err)}
    proofDeadlineBefore,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{"mfa:sensitive:"+beforeRefreshIdentity.SessionID}).Int64()
    if err!=nil || proofDeadlineBefore<=0 {t.Fatal("missing sensitive proof deadline before refresh",err)}
    proofValueBefore,err:=redis.Get(ctx,"mfa:sensitive:"+beforeRefreshIdentity.SessionID).Result();if err!=nil {t.Fatal(err)}
    // Even with an existing sensitive grant, concurrent tests share the same
    // atomic replay boundary and never renew or overwrite that grant.
    concurrentTestCode:=integrationTOTP(t,setup.Secret)
    var testWinners atomic.Int32
    testResults:=make(chan error,2)
    for range 2 {go func(){result,err:=m.TestMFA(ctx,concurrentTestCode);if err==nil && result.Verified {testWinners.Add(1)};testResults<-err}()}
    for range 2 {err:=<-testResults;if err!=nil && !errors.Is(err,auth.ErrMFAInvalid) {t.Fatal(err)}}
    if testWinners.Load()!=1 {t.Fatal("concurrent authenticator tests reused a code",testWinners.Load())}
    proofValueAfter,err:=redis.Get(ctx,"mfa:sensitive:"+beforeRefreshIdentity.SessionID).Result()
    if err!=nil || proofValueAfter!=proofValueBefore {t.Fatal("authenticator test replaced sensitive grant",err)}
    proofDeadlineAfterTest,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{"mfa:sensitive:"+beforeRefreshIdentity.SessionID}).Int64()
    if err!=nil || proofDeadlineAfterTest!=proofDeadlineBefore {t.Fatal("authenticator test renewed sensitive grant",err)}
    refreshed,err:=m.Refresh(t.Context(),loggedIn.RefreshToken);if err!=nil {t.Fatal(err)}
    if deletion(refreshed.AccessToken)!=204 {t.Fatal("refresh lost same-session grant")}
    identity,err:=manager.Parse(refreshed.AccessToken);if err!=nil {t.Fatal(err)}
    ttlBefore,err:=redis.Eval(ctx,`return redis.call('PTTL', KEYS[1])`,[]string{"mfa:sensitive:"+identity.SessionID}).Int64()
    if err!=nil || ttlBefore<=0 || ttlBefore>(90*time.Second).Milliseconds() {t.Fatal("invalid sensitive TTL",ttlBefore,err)}
    proofDeadlineAfter,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{"mfa:sensitive:"+identity.SessionID}).Int64()
    if err!=nil || proofDeadlineAfter!=proofDeadlineBefore {t.Fatal("refresh extended configured sensitive proof deadline",err)}
    auditCtx,auditState:=auditctx.WithState(authenticate(refreshed.AccessToken,"/mfa"))
    request:=httptest.NewRequest("DELETE","/mfa-probe/1",nil).WithContext(auditCtx)
    if err:=m.CheckHTTPSecurity(request);err!=nil {t.Fatal(err)}
    event:=auditState.Snapshot()
    if event.MFAFingerprint=="" || event.MFAVerifiedAt==0 {t.Fatal("missing verification fingerprint")}
    event.Kind="operation";event.Method="DELETE";event.Module="user";event.Operation="delete";event.Status=200;event.OccurredAt=time.Now().UnixMilli()
    recorder:=audit.New(store,limits, config.DefaultAudit())
    if err:=recorder.RecordHTTP(ctx,event);err!=nil {t.Fatal(err)}
    var fingerprint string
    if err:=store.DB.QueryRowContext(ctx,`SELECT mfa_fingerprint FROM t_audit_event ORDER BY id DESC LIMIT 1`).Scan(&fingerprint);err!=nil || fingerprint!=event.MFAFingerprint {t.Fatal("audit fingerprint not persisted",err)}
    // Logout revokes only this session's sensitive grant. Account-wide attempt
    // history and consumed TOTP counters must survive logout and a new login.
    otherCtx:=authenticate(reset.AccessToken,"/mfa")
    usedBeforeLogout:=integrationTOTP(t,setup.Secret)
    if _,err:=m.VerifySensitiveMFA(otherCtx,usedBeforeLogout);err!=nil {t.Fatal("prepare independent session grant",err)}
    otherIdentity,err:=manager.Parse(reset.AccessToken);if err!=nil {t.Fatal(err)}
    otherProofKey:="mfa:sensitive:"+otherIdentity.SessionID
    otherProofBefore,err:=redis.Get(ctx,otherProofKey).Result();if err!=nil {t.Fatal(err)}
    otherDeadlineBefore,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{otherProofKey}).Int64();if err!=nil {t.Fatal(err)}
    attemptsBeforeLogout,err:=redis.Eval(ctx,`return tonumber(redis.call('GET', KEYS[1]) or '0')`,[]string{attemptKey}).Int64();if err!=nil {t.Fatal(err)}
    attemptDeadlineBeforeLogout,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{attemptKey}).Int64();if err!=nil {t.Fatal(err)}
    var usedStepBeforeLogout int64
    if err:=store.DB.QueryRowContext(ctx,`SELECT last_used_step FROM t_user_totp WHERE user_id=101`).Scan(&usedStepBeforeLogout);err!=nil {t.Fatal(err)}
    if loggedIn.RefreshToken==refreshed.RefreshToken {t.Fatal("logout fixture did not rotate the refresh generation")}
    publicLogout:=func(token string)*httptest.ResponseRecorder {
        t.Helper()
        r:=httptest.NewRequest("POST","/pub/logout",strings.NewReader(`{"refresh_token":"`+token+`"}`))
        w:=httptest.NewRecorder();router.ServeHTTP(w,r);return w
    }
    // Any authentic refresh generation identifies the same session family;
    // repeating it must also finish cleanup after the session is already gone.
    for range 2 {
        if w:=publicLogout(loggedIn.RefreshToken);w.Code!=200 || w.Header().Get("Cache-Control")!="no-store" {t.Fatal("public logout with previous generation failed",w.Code)}
        remainingProof,err:=redis.Eval(ctx,`return redis.call('EXISTS', KEYS[1])`,[]string{"mfa:sensitive:"+identity.SessionID}).Int64()
        if err!=nil || remainingProof!=0 {t.Fatal("logout retained sensitive grant",err)}
        attemptsAfterLogout,err:=redis.Eval(ctx,`return tonumber(redis.call('GET', KEYS[1]) or '0')`,[]string{attemptKey}).Int64()
        if err!=nil || attemptsAfterLogout!=attemptsBeforeLogout {t.Fatal("logout changed the shared MFA attempt budget",err)}
        attemptDeadlineAfterLogout,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{attemptKey}).Int64()
        if err!=nil || attemptDeadlineAfterLogout!=attemptDeadlineBeforeLogout {t.Fatal("logout changed the MFA attempt window",err)}
        var usedStepAfterLogout int64
        if err:=store.DB.QueryRowContext(ctx,`SELECT last_used_step FROM t_user_totp WHERE user_id=101`).Scan(&usedStepAfterLogout);err!=nil || usedStepAfterLogout!=usedStepBeforeLogout {t.Fatal("logout changed consumed TOTP state",err)}
        otherProofAfter,err:=redis.Get(ctx,otherProofKey).Result()
        if err!=nil || otherProofAfter!=otherProofBefore {t.Fatal("logout changed another session's sensitive grant",err)}
        otherDeadlineAfter,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{otherProofKey}).Int64()
        if err!=nil || otherDeadlineAfter!=otherDeadlineBefore {t.Fatal("logout changed another session's grant deadline",err)}
    }
    if deletion(loggedIn.AccessToken)!=401 || deletion(refreshed.AccessToken)!=401 {t.Fatal("logout left an old access token usable")}
    if deletion(reset.AccessToken)!=204 {t.Fatal("logout invalidated another session's sensitive authorization")}
    reloginPending:=login("changed")
    if !reloginPending.MFARequired || reloginPending.AccessToken!="" {t.Fatal("logout bypassed login MFA")}
    if _,err:=m.CompleteMFALogin(t.Context(),reloginPending.MFATicket,usedBeforeLogout);!errors.Is(err,auth.ErrMFAInvalid) {t.Fatal("logout allowed a consumed code to log in again",err)}
    relogged,err:=m.CompleteMFALogin(t.Context(),reloginPending.MFATicket,integrationTOTP(t,setup.Secret));if err!=nil {t.Fatal(err)}
    issued=append(issued,relogged)
    reloggedIdentity,err:=manager.Parse(relogged.AccessToken);if err!=nil {t.Fatal(err)}
    if reloggedIdentity.SessionID==identity.SessionID || reloggedIdentity.CredentialVersion!=identity.CredentialVersion {t.Fatal("relogin reused the logged-out session or reset credential version")}
    reloggedCtx:=authenticate(relogged.AccessToken,"/mfa")
    reloggedStatus,err:=m.MFAStatus(reloggedCtx)
    if err!=nil || !reloggedStatus.Enabled || reloggedStatus.SensitiveExpiredAt!=0 {t.Fatal("relogin inherited a sensitive grant",err)}
    mutationsBeforeReloggedOperation:=probe.mutations.Load()
    firstReloggedOperation:=httptest.NewRequest("DELETE","/mfa-probe/1",nil)
    firstReloggedOperation.Header.Set("Authorization","Bearer "+relogged.AccessToken)
    firstReloggedResponse:=httptest.NewRecorder();router.ServeHTTP(firstReloggedResponse,firstReloggedOperation)
    if firstReloggedResponse.Code!=403 || firstReloggedResponse.Header().Get(authn.MFARequiredHeader)!="true" || probe.mutations.Load()!=mutationsBeforeReloggedOperation {t.Fatal("first sensitive operation after relogin bypassed verification")}
    // The recovery checks below must invalidate this genuinely new session,
    // rather than asserting against credentials already invalidated by logout.
    loggedIn,refreshed,ctx=relogged,relogged,reloggedCtx
    // Recovery is a separate password-verified transition, never a login or proof.
    recoveryA,recoveryB:=login("changed"),login("changed")
    var beforeCount,beforeLoggedAt,beforeVersion int64
    if err:=store.DB.QueryRowContext(ctx,`SELECT login_count,last_logged_in_at,credential_version FROM t_user WHERE id=101`).Scan(&beforeCount,&beforeLoggedAt,&beforeVersion);err!=nil {t.Fatal(err)}
    type recoveryOutcome struct {result *auth.LoginResult; err error}
    recoveredResults:=make(chan recoveryOutcome,2)
    for _,ticket:=range []string{recoveryA.MFATicket,recoveryB.MFATicket} {
        go func(ticket string){result,err:=m.RecoverMFALogin(t.Context(),ticket,enrolled.RecoveryCodes[1]);recoveredResults<-recoveryOutcome{result,err}}(ticket)
    }
    var recovered *auth.LoginResult
    for range 2 {
        outcome:=<-recoveredResults
        if outcome.err==nil {if recovered!=nil {t.Fatal("concurrent recovery succeeded twice")};recovered=outcome.result;issued=append(issued,recovered)} else if !errors.Is(outcome.err,auth.ErrUnauthorized) && !errors.Is(outcome.err,auth.ErrMFAInvalid) {t.Fatal(outcome.err)}
    }
    if recovered==nil || !recovered.MFAEnrollmentRequired || recovered.PasswordResetRequired {t.Fatal("recovery did not issue only mandatory enrollment")}
    newAttemptKey:=fmt.Sprintf("mfa:attempt:101:%d",beforeVersion+1)
    freshAttempts,err:=redis.Eval(ctx,`return tonumber(redis.call('GET', KEYS[1]) or '0')`,[]string{newAttemptKey}).Int64()
    if err!=nil || freshAttempts!=0 {t.Fatal("successful recovery did not start with zero attempts",freshAttempts,err)}
    var afterCount,afterLoggedAt,afterVersion int64
    if err:=store.DB.QueryRowContext(ctx,`SELECT login_count,last_logged_in_at,credential_version FROM t_user WHERE id=101`).Scan(&afterCount,&afterLoggedAt,&afterVersion);err!=nil || afterCount!=beforeCount || afterLoggedAt!=beforeLoggedAt || afterVersion!=beforeVersion+1 {t.Fatal("recovery completed a login or changed version incorrectly",err)}
    var remaining int
    if err:=store.DB.QueryRowContext(ctx,`SELECT (SELECT COUNT(*) FROM t_user_totp WHERE user_id=101)+(SELECT COUNT(*) FROM t_user_recovery_code WHERE user_id=101)`).Scan(&remaining);err!=nil || remaining!=0 {t.Fatal("recovery left old factor or codes",err)}
    if deletion(refreshed.AccessToken)!=401 || deletion(loggedIn.AccessToken)!=401 {t.Fatal("recovery left old sessions or sensitive proof usable")}
    recoveredCtx:=authenticate(recovered.AccessToken,"/mfa/setup")
    if _,err:=m.VerifySensitiveMFA(recoveredCtx,integrationTOTP(t,setup.Secret));!errors.Is(err,auth.ErrMFAEnrollmentRequired) {t.Fatal("old factor granted recovered session sensitive access",err)}
    for _,target:=range []struct{method,path string}{ {"GET","/info"},{"DELETE","/mfa-probe/1"}} {
        r:=httptest.NewRequest(target.method,target.path,nil);r.Header.Set("Authorization","Bearer "+recovered.AccessToken)
        w:=httptest.NewRecorder();router.ServeHTTP(w,r)
        if w.Code!=403 || w.Header().Get(authn.MFAEnrollmentRequiredHeader)!="true" {t.Fatal("recovery bypassed mandatory binding",target.path,w.Code)}
    }
    recoveredRefresh,err:=m.Refresh(t.Context(),recovered.RefreshToken);if err!=nil || !recoveredRefresh.MFAEnrollmentRequired {t.Fatal("refresh upgraded recovery restrictions",err)}
    recoveredCtx=authenticate(recoveredRefresh.AccessToken,"/mfa/setup")
    status,err=m.MFAStatus(recoveredCtx);if err!=nil || status.PasswordRequired || !status.EnrollmentRequired || status.SensitiveExpiredAt!=0 {t.Fatal("recovery grant/proof state incorrect",err)}
    reboundSetup,err:=m.BeginMFAEnrollment(recoveredCtx,"");if err!=nil {t.Fatal(err)}
    rebound,err:=m.ConfirmMFAEnrollment(recoveredCtx,integrationTOTP(t,reboundSetup.Secret));if err!=nil {t.Fatal(err)}
    issued=append(issued,rebound.LoginResult)
    if rebound.MFAEnrollmentRequired || len(rebound.RecoveryCodes)!=10 || reboundSetup.Secret==setup.Secret {t.Fatal("recovery did not bind a fresh factor")}
    if deletion(recoveredRefresh.AccessToken)!=401 || deletion(rebound.AccessToken)!=403 {t.Fatal("rebind did not replace recovery session without a proof")}
    oldRecoveryTicket:=login("changed")
    if _,err:=m.RecoverMFALogin(t.Context(),oldRecoveryTicket.MFATicket,enrolled.RecoveryCodes[1]);!errors.Is(err,auth.ErrMFAInvalid) {t.Fatal("old recovery code survived rebind",err)}
    ctx=authenticate(rebound.AccessToken,"/mfa")
    refreshed=rebound.LoginResult
    if err:=m.ResetUserMFA(ctx,101);err!=nil {t.Fatal(err)}
    if deletion(refreshed.AccessToken)!=401 {t.Fatal("administrator reset left session valid")}
    var count int
    if err:=store.DB.QueryRowContext(ctx,`SELECT (SELECT COUNT(*) FROM t_user_totp WHERE user_id=101)+(SELECT COUNT(*) FROM t_user_recovery_code WHERE user_id=101)`).Scan(&count);err!=nil || count!=0 {t.Fatal("reset left credentials",err)}
    if _,err:=m.VerifySensitiveMFA(ctx,integrationTOTP(t,reboundSetup.Secret));!errors.Is(err,auth.ErrUnauthorized) {t.Fatal("old identity or code survived reset",err)}
}
{{ end -}}
