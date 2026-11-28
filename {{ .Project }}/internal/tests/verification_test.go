package tests

{{ if .Computed.enable_redis_final -}}
import (
    "context"
    "crypto/hmac"
    "crypto/sha1"
    "encoding/base32"
    "encoding/binary"
    "encoding/json"
    "github.com/go-jose/go-jose/v4"
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

type verificationProbe struct { mutations atomic.Int32 }
func (*verificationProbe) Name() string {return "/totp-probe"}
{{ if eq .Computed.http_router_final "gin" }}
func (m *verificationProbe) HTTP(r *gin.RouterGroup) {
    r.DELETE("/:id",func(c *gin.Context){m.mutations.Add(1);c.Status(http.StatusNoContent)})
}
{{ else }}
func (m *verificationProbe) HTTP() http.Handler {
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

func TestTOTPRealDatabaseRedisLifecycle(t *testing.T) {
    store:=permissionDatabase(t)
    redis:=sessionRedisClient(t)
    localizer:=testLocalization(t,store)
    limits,_:=pagination.New(100,100)
    dictionaries,err:=dictionary.New(store,dictionary.NewMemoryValueCache(),dictionary.CacheTTL,limits,localizer)
    if err!=nil {t.Fatal(err)}
    captcha,err:=auth.NewPointCaptcha(auth.NewMemoryPointCaptchaStore(),dictionaries,5,time.Minute,300,180,22,false)
    if err!=nil {t.Fatal(err)}
    slider,err:=auth.NewSliderCaptcha(auth.NewMemoryPointCaptchaStore(),auth.SliderCaptchaConfig{TTL:time.Minute,MinimumDuration:100*time.Millisecond})
    if err!=nil {t.Fatal(err)}
    guard,err:=auth.NewPasswordChangeGuard(auth.NewMemoryPasswordFailureStore(),captcha,3,6,time.Minute)
    if err!=nil {t.Fatal(err)}
    key:=strings.Repeat("k",64)
    sessions,err:=auth.NewSessions(auth.NewRedisSessionStore(redis),auth.SessionConfig{Issuer:"totp-test",Key:key,SessionLifetime:time.Hour,RememberLifetime:24*time.Hour,RetryWindow:time.Second})
    if err!=nil {t.Fatal(err)}
    manager,err:=authn.New("totp-test",key,time.Hour,auth.NewIdentityValidator(store,sessions))
    if err!=nil {t.Fatal(err)}
    var issued []*auth.LoginResult
    var tickets []string
    // Register before the first mutation so assertion failures also clean Redis.
    t.Cleanup(func(){
        cleanupCtx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
        for _,result:=range issued {
            id,err:=manager.Parse(result.AccessToken);if err==nil {
                if err:=sessions.Revoke(cleanupCtx,id.SessionID);err!=nil {t.Error(err)}
                if err:=redis.Del(cleanupCtx,"verification:enrollment:"+id.SessionID,"verification:sensitive:"+id.SessionID,"verification:enrollment-grant:"+id.SessionID,fmt.Sprintf("verification:attempt:%d:%d",id.UserID,id.CredentialVersion)).Err();err!=nil {t.Error(err)}
            }
        }
        for _,ticket:=range tickets {if err:=redis.Del(cleanupCtx,"verification:login:v2:"+ticket).Err();err!=nil {t.Error(err)}}
    })
    // This lifecycle deliberately exercises many valid and invalid Verification flows;
    // dedicated rate-limit tests cover the bounded fixed-window policy.
    verification,err:=auth.NewVerification(store,auth.NewRedisVerificationStore(redis),auth.VerificationConfig{TOTPEnabled: true, LoginTicketTTL: 5*time.Minute, TOTPPeriod:time.Second,TOTPEnrollmentTTL:5*time.Minute,SensitiveLifetime:90*time.Second,RateLimitWindow:5*time.Minute,RateLimitMaxAttempts:32,Issuer:"totp-test",SigningKey:key})
    if err!=nil {t.Fatal(err)}
    credentials,err:=auth.NewCredentials("test","",time.Minute,auth.NewMemoryChallengeStore())
    if err!=nil { t.Fatal(err) }
    m:=auth.New(store,manager,credentials,sessions,captcha,slider,guard,localizer,nil,verification)
    manager.SetHTTPSecurityPolicy(m.CheckHTTPSecurity)
    hash,err:=bcrypt.GenerateFromPassword([]byte("initial"),bcrypt.MinCost);if err!=nil {t.Fatal(err)}
    if _,err:=store.DB.ExecContext(t.Context(),`INSERT INTO t_user (id,username,code,password,status,role_id) VALUES (101,'verification-user','MFATEST1',$1,1,1)`,string(hash));err!=nil {t.Fatal(err)}
    login:=func(password string)*auth.LoginResult {
        t.Helper();input,err:=sliderLoginInput(t.Context(),slider,"verification-user",password,false);if err!=nil {t.Fatal(err)}
        result,err:=m.Login(t.Context(),input);if err!=nil {t.Fatal(err)};if result.AccessToken!="" {issued=append(issued,result)};if result.MFATicket!="" {tickets=append(tickets,result.MFATicket)};return result
    }
    authenticate:=func(token,path string)context.Context {
        t.Helper();method:="POST";if path=="/reset/pwd" {method="PATCH"};r:=httptest.NewRequest(method,path,nil);r.Header.Set("Authorization","Bearer "+token)
        request,err:=manager.AuthenticateRequest(r);if err!=nil {t.Fatal(err)};return request.Context()
    }
    sink:=&emailSink{}
    codes,err:=newTemplateEmailCodes(store,sink,auth.EmailConfig{CodeTTL:time.Minute,ResendInterval:time.Minute,SendLimit:3,MaxAttempts:4},key)
    if err!=nil {t.Fatal(err)}
    m.SetEmailCodes(codes)
    if _,err=store.DB.ExecContext(t.Context(),`UPDATE t_user SET email='verification@example.test',email_verified_at=1 WHERE id=101`);err!=nil {t.Fatal(err)}

    stopWorkers, err := codes.StartWorkers(t.Context())
    if err != nil { t.Fatal(err) }
    t.Cleanup(stopWorkers)
    first:=login("initial")
    if !first.PasswordResetRequired || !first.TOTPEnrollmentRequired {t.Fatal("first login must require both Verification enrollment and password reset")}
    resetRequest:=httptest.NewRequest("PATCH","/reset/pwd",nil)
    resetRequest.Header.Set("Authorization","Bearer "+first.AccessToken)
    authenticatedReset,err:=manager.AuthenticateRequest(resetRequest);if err!=nil {t.Fatal(err)}
    if err:=manager.CheckHTTPSecurity(authenticatedReset);!errors.Is(err,auth.ErrTOTPEnrollmentRequired) {t.Fatal("password reset bypassed Verification enrollment",err)}
    restrictedCtx:=authenticate(first.AccessToken,"/totp/setup")
    if _,err:=m.ResetPassword(restrictedCtx,auth.PasswordResetInput{NewPassword:"changed"});!errors.Is(err,auth.ErrTOTPEnrollmentRequired) {t.Fatal("direct password reset bypassed Verification enrollment",err)}
    probe:=&verificationProbe{}
    router,err:=server.NewRouter(&config.Config{},manager,nil,m,probe)
    if err!=nil {t.Fatal(err)}
    deletion:=func(token string)int {
        t.Helper();r:=httptest.NewRequest("DELETE","/totp-probe/1",nil);r.Header.Set("Authorization","Bearer "+token)
        response:=httptest.NewRecorder();router.ServeHTTP(response,r);return response.Code
    }
    if deletion(first.AccessToken)!=403 || probe.mutations.Load()!=0 {t.Fatal("mandatory binding bypassed")}
    status,err:=m.TOTPStatus(restrictedCtx);if err!=nil || status.PasswordRequired || !status.EnrollmentRequired {t.Fatal("login did not authorize direct enrollment",err)}
    refreshedFirst,err:=m.Refresh(t.Context(),first.RefreshToken);if err!=nil {t.Fatal(err)}
    issued=append(issued,refreshedFirst)
    if !refreshedFirst.TOTPEnrollmentRequired || !refreshedFirst.PasswordResetRequired {t.Fatal("refresh lost onboarding restrictions")}
    restrictedCtx=authenticate(refreshedFirst.AccessToken,"/totp/setup")
    resumed:=login("initial")
    resumedStatus,err:=m.TOTPStatus(authenticate(resumed.AccessToken,"/totp/setup"))
    if err!=nil || resumedStatus.PasswordRequired {t.Fatal("password login did not authorize mandatory enrollment",err)}
    setup,err:=m.BeginTOTPEnrollment(restrictedCtx,"");if err!=nil {t.Fatal(err)}
    enrolled,err:=m.ConfirmTOTPEnrollment(restrictedCtx,integrationTOTP(t,setup.Secret));if err!=nil {t.Fatal(err)}
    issued=append(issued,enrolled.LoginResult)
    if enrolled.TOTPEnrollmentRequired || !enrolled.PasswordResetRequired {t.Fatal("binding must preserve password reset requirement")}
    if deletion(first.AccessToken)!=401 {t.Fatal("binding did not invalidate prior session")}
    // Already-bound first-login accounts must verify Verification before password reset too.
    pendingReset:=login("initial")
    if !pendingReset.MFARequired || pendingReset.AccessToken!="" {t.Fatal("bound first login bypassed Verification")}
    verifiedReset,err:=m.CompleteTOTPLogin(t.Context(),pendingReset.MFATicket,integrationTOTP(t,setup.Secret));if err!=nil {t.Fatal(err)}
    issued=append(issued,verifiedReset)
    if !verifiedReset.PasswordResetRequired || verifiedReset.TOTPEnrollmentRequired {t.Fatal("wrong state after login verification")}
    reset,err:=m.ResetPassword(authenticate(verifiedReset.AccessToken,"/reset/pwd"),auth.PasswordResetInput{NewPassword:"changed"})
    if err!=nil || reset.TOTPEnrollmentRequired || reset.PasswordResetRequired {t.Fatal("password reset did not finish onboarding",err)}
    issued=append(issued,reset)
    for _, tc := range []struct {
        result *auth.LoginResult
        second authn.Method
    }{
        {first, ""}, {refreshedFirst, ""}, {enrolled.LoginResult, authn.TOTP},
        {verifiedReset, authn.TOTP}, {reset, authn.TOTP},
    } {
        id, err := manager.Parse(tc.result.AccessToken)
        if err != nil || id.Methods != (authn.Methods{Primary:authn.Password, SecondFactor:tc.second}) {
            t.Fatalf("unexpected onboarding methods: %+v, %v", id.Methods, err)
        }
    }
    var stored string
    if err:=store.DB.QueryRowContext(t.Context(),`SELECT secret_ciphertext FROM t_user_totp WHERE user_id=101`).Scan(&stored);err!=nil {t.Fatal(err)}
    if strings.Contains(stored,setup.Secret) {t.Fatal("plaintext TOTP secret stored")}
    var storedPeriod int64
    if err := store.DB.QueryRowContext(t.Context(), `SELECT period_seconds FROM t_user_totp WHERE user_id=101`).Scan(&storedPeriod); err != nil || storedPeriod != 1 { t.Fatal("enrollment period was not persisted", storedPeriod, err) }
    // A newly constructed verifier simulates restarting with a different env default.
    restartedVerification, err := auth.NewVerification(store, auth.NewRedisVerificationStore(redis), auth.VerificationConfig{TOTPEnabled:true, LoginTicketTTL:5*time.Minute, TOTPPeriod:30*time.Second, TOTPEnrollmentTTL:5*time.Minute, SensitiveLifetime:90*time.Second, RateLimitWindow:5*time.Minute, RateLimitMaxAttempts:32, Issuer:"totp-test", SigningKey:key})
    if err != nil { t.Fatal(err) }
    restarted := auth.New(store, manager, credentials, sessions, captcha, slider, guard, localizer, nil, restartedVerification)
    restartCode := integrationTOTP(t, setup.Secret)
    if _, err := restarted.TestTOTP(authenticate(reset.AccessToken, "/totp/test"), restartCode); err != nil { t.Fatal("restart ignored stored binding period", err) }
    if _, err := restarted.TestTOTP(authenticate(reset.AccessToken, "/totp/test"), restartCode); !errors.Is(err, auth.ErrVerificationInvalid) { t.Fatal("restart allowed code replay", err) }

    if deletion(enrolled.AccessToken)!=401 {t.Fatal("password reset did not invalidate prior session")}
    if deletion(reset.AccessToken)!=403 {t.Fatal("enrollment created sensitive grant")}
    pending:=login("changed")
    if !pending.MFARequired || pending.AccessToken!="" || pending.RefreshToken!="" {t.Fatal("password-only login returned session")}
    loggedIn,err:=m.CompleteTOTPLogin(t.Context(),pending.MFATicket,integrationTOTP(t,setup.Secret));if err!=nil {t.Fatal(err)}
    issued=append(issued,loggedIn)
    if deletion(loggedIn.AccessToken)!=403 {t.Fatal("login granted sensitive access")}
    if _,err:=m.CompleteTOTPLogin(t.Context(),pending.MFATicket,"123456");!errors.Is(err,auth.ErrVerificationInvalid) {t.Fatal("login ticket replay accepted",err)}
    testCode:=func(code string)*httptest.ResponseRecorder {
        r:=httptest.NewRequest("POST","/totp/test",strings.NewReader(`{"verification_code":"`+code+`"}`))
        r.Header.Set("Authorization","Bearer "+loggedIn.AccessToken)
        w:=httptest.NewRecorder();router.ServeHTTP(w,r);return w
    }
    testIdentity,err:=manager.Parse(loggedIn.AccessToken);if err!=nil {t.Fatal(err)}
    attemptKey:=fmt.Sprintf("verification:attempt:%d:%d",testIdentity.UserID,testIdentity.CredentialVersion)
    testAttemptsBefore,err:=redis.Eval(t.Context(),`return tonumber(redis.call('GET', KEYS[1]) or '0')`,[]string{attemptKey}).Int64();if err!=nil {t.Fatal(err)}
    testedCode:=integrationTOTP(t,setup.Secret)
    if w:=testCode(testedCode);w.Code!=200 || !strings.Contains(w.Body.String(),`"verified":true`) || w.Header().Get(authn.VerificationRequiredHeader)!="" {t.Fatal("authenticator test failed",w.Code)}
    if w:=testCode(testedCode);w.Code!=400 || w.Header().Get(authn.VerificationRequiredHeader)!="" {t.Fatal("test code replay accepted or requested sensitive verification",w.Code)}
    testAttemptsAfter,err:=redis.Eval(t.Context(),`return tonumber(redis.call('GET', KEYS[1]) or '0')`,[]string{attemptKey}).Int64()
    if err!=nil || testAttemptsAfter!=testAttemptsBefore+2 {t.Fatal("successful and replayed tests did not share attempt budget",err)}
    if deletion(loggedIn.AccessToken)!=403 {t.Fatal("authenticator test created sensitive grant")}
    testLogin:=login("changed")
    if _,err:=m.CompleteTOTPLogin(t.Context(),testLogin.MFATicket,testedCode);!errors.Is(err,auth.ErrVerificationInvalid) {t.Fatal("tested code reused for login",err)}
    if _,err:=m.VerifySensitiveTOTP(authenticate(loggedIn.AccessToken,"/totp/test"),testedCode);!errors.Is(err,auth.ErrVerificationInvalid) {t.Fatal("tested code reused for sensitive verification",err)}
    inlineDelete:=func(code string)*httptest.ResponseRecorder {
        r:=httptest.NewRequest("DELETE","/totp-probe/1",nil)
        r.Header.Set("Authorization","Bearer "+loggedIn.AccessToken)
        r.Header.Set(authn.TOTPCodeHeader,code)
        w:=httptest.NewRecorder();router.ServeHTTP(w,r);return w
    }
    beforeMutations:=probe.mutations.Load()
    for _,code:=range []string{"","invalid-recovery-code",enrolled.RecoveryCodes[4]} {
        w:=inlineDelete(code)
        if (w.Code!=403 && w.Code!=400) || w.Header().Get(authn.VerificationRequiredHeader)!="true" || probe.mutations.Load()!=beforeMutations {t.Fatal("inline Verification gate bypass",w.Code)}
    }
    if w:=inlineDelete(integrationTOTP(t,setup.Secret));w.Code!=204 || w.Header().Get(authn.VerificationRequiredHeader)!="" || probe.mutations.Load()!=beforeMutations+1 {t.Fatal("same-endpoint Verification did not execute mutation",w.Code)}
    ctx:=authenticate(loggedIn.AccessToken,"/totp")
    // The same TOTP step submitted concurrently must produce exactly one grant.
    concurrentCode:=integrationTOTP(t,setup.Secret)
    var winners atomic.Int32
    results:=make(chan error,2)
    for range 2 {go func(){_,err:=m.VerifySensitiveTOTP(ctx,concurrentCode);if err==nil {winners.Add(1)};results<-err}()}
    for range 2 {err:=<-results;if err!=nil && !errors.Is(err,auth.ErrVerificationInvalid) {t.Fatal(err)}}
    if winners.Load()!=1 {t.Fatal("non-atomic TOTP consumption",winners.Load())}
    if deletion(loggedIn.AccessToken)!=204 || deletion(loggedIn.AccessToken)!=204 {t.Fatal("valid sensitive window rejected")}
    if deletion(reset.AccessToken)!=403 {t.Fatal("sensitive grant leaked to another session")}
    beforeRefreshIdentity,err:=manager.Parse(loggedIn.AccessToken);if err!=nil {t.Fatal(err)}
    proofDeadlineBefore,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{"verification:sensitive:"+beforeRefreshIdentity.SessionID}).Int64()
    if err!=nil || proofDeadlineBefore<=0 {t.Fatal("missing sensitive proof deadline before refresh",err)}
    proofValueBefore,err:=redis.Get(ctx,"verification:sensitive:"+beforeRefreshIdentity.SessionID).Result();if err!=nil {t.Fatal(err)}
    // Even with an existing sensitive grant, concurrent tests share the same
    // atomic replay boundary and never renew or overwrite that grant.
    concurrentTestCode:=integrationTOTP(t,setup.Secret)
    var testWinners atomic.Int32
    testResults:=make(chan error,2)
    for range 2 {go func(){result,err:=m.TestTOTP(ctx,concurrentTestCode);if err==nil && result.Verified {testWinners.Add(1)};testResults<-err}()}
    for range 2 {err:=<-testResults;if err!=nil && !errors.Is(err,auth.ErrVerificationInvalid) {t.Fatal(err)}}
    if testWinners.Load()!=1 {t.Fatal("concurrent authenticator tests reused a code",testWinners.Load())}
    proofValueAfter,err:=redis.Get(ctx,"verification:sensitive:"+beforeRefreshIdentity.SessionID).Result()
    if err!=nil || proofValueAfter!=proofValueBefore {t.Fatal("authenticator test replaced sensitive grant",err)}
    proofDeadlineAfterTest,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{"verification:sensitive:"+beforeRefreshIdentity.SessionID}).Int64()
    if err!=nil || proofDeadlineAfterTest!=proofDeadlineBefore {t.Fatal("authenticator test renewed sensitive grant",err)}
    refreshed,err:=m.Refresh(t.Context(),loggedIn.RefreshToken);if err!=nil {t.Fatal(err)}
    if deletion(refreshed.AccessToken)!=204 {t.Fatal("refresh lost same-session grant")}
    identity,err:=manager.Parse(refreshed.AccessToken);if err!=nil {t.Fatal(err)}
    if identity.Methods != (authn.Methods{Primary:authn.Password, SecondFactor:authn.TOTP}) {t.Fatal("refresh changed login methods",identity.Methods)}
    ttlBefore,err:=redis.Eval(ctx,`return redis.call('PTTL', KEYS[1])`,[]string{"verification:sensitive:"+identity.SessionID}).Int64()
    if err!=nil || ttlBefore<=0 || ttlBefore>(90*time.Second).Milliseconds() {t.Fatal("invalid sensitive TTL",ttlBefore,err)}
    proofDeadlineAfter,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{"verification:sensitive:"+identity.SessionID}).Int64()
    if err!=nil || proofDeadlineAfter!=proofDeadlineBefore {t.Fatal("refresh extended configured sensitive proof deadline",err)}
    auditCtx,auditState:=auditctx.WithState(authenticate(refreshed.AccessToken,"/totp"))
    request:=httptest.NewRequest("DELETE","/totp-probe/1",nil).WithContext(auditCtx)
    if err:=m.CheckHTTPSecurity(request);err!=nil {t.Fatal(err)}
    event:=auditState.Snapshot()
    if event.StepUpFingerprint=="" || event.StepUpVerifiedAt==0 {t.Fatal("missing verification fingerprint")}
    event.Kind="operation";event.Method="DELETE";event.Module="user";event.Operation="delete";event.Status=200;event.OccurredAt=time.Now().UnixMilli()
    recorder:=audit.New(store,limits, config.DefaultAudit())
    if err:=recorder.RecordHTTP(ctx,event);err!=nil {t.Fatal(err)}
    var fingerprint string
    if err:=store.DB.QueryRowContext(ctx,`SELECT step_up_fingerprint FROM t_audit_event ORDER BY id DESC LIMIT 1`).Scan(&fingerprint);err!=nil || fingerprint!=event.StepUpFingerprint {t.Fatal("audit fingerprint not persisted",err)}
    // Logout revokes only this session's sensitive grant. Account-wide attempt
    // history and consumed TOTP counters must survive logout and a new login.
    otherCtx:=authenticate(reset.AccessToken,"/totp")
    usedBeforeLogout:=integrationTOTP(t,setup.Secret)
    if _,err:=m.VerifySensitiveTOTP(otherCtx,usedBeforeLogout);err!=nil {t.Fatal("prepare independent session grant",err)}
    otherIdentity,err:=manager.Parse(reset.AccessToken);if err!=nil {t.Fatal(err)}
    otherProofKey:="verification:sensitive:"+otherIdentity.SessionID
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
        remainingProof,err:=redis.Eval(ctx,`return redis.call('EXISTS', KEYS[1])`,[]string{"verification:sensitive:"+identity.SessionID}).Int64()
        if err!=nil || remainingProof!=0 {t.Fatal("logout retained sensitive grant",err)}
        attemptsAfterLogout,err:=redis.Eval(ctx,`return tonumber(redis.call('GET', KEYS[1]) or '0')`,[]string{attemptKey}).Int64()
        if err!=nil || attemptsAfterLogout!=attemptsBeforeLogout {t.Fatal("logout changed the shared Verification attempt budget",err)}
        attemptDeadlineAfterLogout,err:=redis.Eval(ctx,`return redis.call('PEXPIRETIME', KEYS[1])`,[]string{attemptKey}).Int64()
        if err!=nil || attemptDeadlineAfterLogout!=attemptDeadlineBeforeLogout {t.Fatal("logout changed the Verification attempt window",err)}
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
    if !reloginPending.MFARequired || reloginPending.AccessToken!="" {t.Fatal("logout bypassed login Verification")}
    if _,err:=m.CompleteTOTPLogin(t.Context(),reloginPending.MFATicket,usedBeforeLogout);!errors.Is(err,auth.ErrVerificationInvalid) {t.Fatal("logout allowed a consumed code to log in again",err)}
    relogged,err:=m.CompleteTOTPLogin(t.Context(),reloginPending.MFATicket,integrationTOTP(t,setup.Secret));if err!=nil {t.Fatal(err)}
    issued=append(issued,relogged)
    reloggedIdentity,err:=manager.Parse(relogged.AccessToken);if err!=nil {t.Fatal(err)}
    if reloggedIdentity.SessionID==identity.SessionID || reloggedIdentity.CredentialVersion!=identity.CredentialVersion {t.Fatal("relogin reused the logged-out session or reset credential version")}
    reloggedCtx:=authenticate(relogged.AccessToken,"/totp")
    reloggedStatus,err:=m.TOTPStatus(reloggedCtx)
    if err!=nil || !reloggedStatus.Enabled {t.Fatal("relogin inherited a sensitive grant",err)}
    mutationsBeforeReloggedOperation:=probe.mutations.Load()
    firstReloggedOperation:=httptest.NewRequest("DELETE","/totp-probe/1",nil)
    firstReloggedOperation.Header.Set("Authorization","Bearer "+relogged.AccessToken)
    firstReloggedResponse:=httptest.NewRecorder();router.ServeHTTP(firstReloggedResponse,firstReloggedOperation)
    if firstReloggedResponse.Code!=403 || firstReloggedResponse.Header().Get(authn.VerificationRequiredHeader)!="true" || probe.mutations.Load()!=mutationsBeforeReloggedOperation {t.Fatal("first sensitive operation after relogin bypassed verification")}
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
        go func(ticket string){result,err:=m.RecoverTOTPLogin(t.Context(),ticket,enrolled.RecoveryCodes[1]);recoveredResults<-recoveryOutcome{result,err}}(ticket)
    }
    var recovered *auth.LoginResult
    for range 2 {
        outcome:=<-recoveredResults
        if outcome.err==nil {if recovered!=nil {t.Fatal("concurrent recovery succeeded twice")};recovered=outcome.result;issued=append(issued,recovered)} else if !errors.Is(outcome.err,auth.ErrUnauthorized) && !errors.Is(outcome.err,auth.ErrVerificationInvalid) {t.Fatal(outcome.err)}
    }
    if recovered==nil || !recovered.TOTPEnrollmentRequired || recovered.PasswordResetRequired {t.Fatal("recovery did not issue only mandatory enrollment")}
    newAttemptKey:=fmt.Sprintf("verification:attempt:101:%d",beforeVersion+1)
    freshAttempts,err:=redis.Eval(ctx,`return tonumber(redis.call('GET', KEYS[1]) or '0')`,[]string{newAttemptKey}).Int64()
    if err!=nil || freshAttempts!=0 {t.Fatal("successful recovery did not start with zero attempts",freshAttempts,err)}
    var afterCount,afterLoggedAt,afterVersion int64
    if err:=store.DB.QueryRowContext(ctx,`SELECT login_count,last_logged_in_at,credential_version FROM t_user WHERE id=101`).Scan(&afterCount,&afterLoggedAt,&afterVersion);err!=nil || afterCount!=beforeCount || afterLoggedAt!=beforeLoggedAt || afterVersion!=beforeVersion+1 {t.Fatal("recovery completed a login or changed version incorrectly",err)}
    var remaining int
    if err:=store.DB.QueryRowContext(ctx,`SELECT (SELECT COUNT(*) FROM t_user_totp WHERE user_id=101)+(SELECT COUNT(*) FROM t_user_recovery_code WHERE user_id=101)`).Scan(&remaining);err!=nil || remaining!=0 {t.Fatal("recovery left old factor or codes",err)}
    if deletion(refreshed.AccessToken)!=401 || deletion(loggedIn.AccessToken)!=401 {t.Fatal("recovery left old sessions or sensitive proof usable")}
    recoveredCtx:=authenticate(recovered.AccessToken,"/totp/setup")
    if _,err:=m.VerifySensitiveTOTP(recoveredCtx,integrationTOTP(t,setup.Secret));!errors.Is(err,auth.ErrTOTPEnrollmentRequired) {t.Fatal("old factor granted recovered session sensitive access",err)}
    for _,target:=range []struct{method,path string}{ {"GET","/info"},{"DELETE","/totp-probe/1"}} {
        r:=httptest.NewRequest(target.method,target.path,nil);r.Header.Set("Authorization","Bearer "+recovered.AccessToken)
        w:=httptest.NewRecorder();router.ServeHTTP(w,r)
        if w.Code!=403 || w.Header().Get("X-MFA-Enrollment-Required")!="" {t.Fatal("recovery bypassed mandatory binding",target.path,w.Code)}
    }
    recoveredRefresh,err:=m.Refresh(t.Context(),recovered.RefreshToken);if err!=nil || !recoveredRefresh.TOTPEnrollmentRequired {t.Fatal("refresh upgraded recovery restrictions",err)}
    recoveredCtx=authenticate(recoveredRefresh.AccessToken,"/totp/setup")
    status,err=m.TOTPStatus(recoveredCtx);if err!=nil || status.PasswordRequired || !status.EnrollmentRequired {t.Fatal("recovery grant/proof state incorrect",err)}
    reboundSetup,err:=m.BeginTOTPEnrollment(recoveredCtx,"");if err!=nil {t.Fatal(err)}
    rebound,err:=m.ConfirmTOTPEnrollment(recoveredCtx,integrationTOTP(t,reboundSetup.Secret));if err!=nil {t.Fatal(err)}
    issued=append(issued,rebound.LoginResult)
    if rebound.TOTPEnrollmentRequired || len(rebound.RecoveryCodes)!=10 || reboundSetup.Secret==setup.Secret {t.Fatal("recovery did not bind a fresh factor")}
    if deletion(recoveredRefresh.AccessToken)!=401 || deletion(rebound.AccessToken)!=403 {t.Fatal("rebind did not replace recovery session without a proof")}
    oldRecoveryTicket:=login("changed")
    if _,err:=m.RecoverTOTPLogin(t.Context(),oldRecoveryTicket.MFATicket,enrolled.RecoveryCodes[1]);!errors.Is(err,auth.ErrVerificationInvalid) {t.Fatal("old recovery code survived rebind",err)}
    // Both entry points confirm only the new factor. Verify notification selection before the email recovery.
    checkNotice:=func(purpose string, want int){
        t.Helper()
        var n int
        if err:=store.DB.QueryRowContext(ctx,`SELECT count(*) FROM t_auth_email_code WHERE user_id=101 AND recipient='verification@example.test' AND purpose=$1`,purpose).Scan(&n);err!=nil || n!=want {t.Fatal(purpose,n,err)}
    }
    checkNotice("totp_bound",1);checkNotice("totp_rebound",1)
    mailTicket:=login("changed").MFATicket
    const uuid="86eefc2b-1e8d-4446-9f69-3f2bd62d8a75"
    sendReset:=func(ticket,pwd string,want int)*httptest.ResponseRecorder {
        return resetMailRequest(t,router,ticket,"email-code",map[string]string{"method":"password","username":"verification-user","password":pwd,"browser_uuid":uuid},want)
    }
    sendReset(mailTicket,"incorrect",400)
    sent:=sendReset(mailTicket,"changed",200)
    var mail auth.EmailCodeStatus
    if err:=json.Unmarshal(sent.Body.Bytes(),&mail);err!=nil || mail.VerificationID=="" {t.Fatal("missing reset ticket",err)}
    sendReset(mailTicket,"changed",429)
    before:=time.Now().Add(3*time.Second)
    for {
        var delivered bool
        if err:=store.DB.QueryRowContext(ctx,`SELECT delivered FROM t_auth_email_code WHERE verification_id=$1 AND purpose='totp_reset'`,mail.VerificationID).Scan(&delivered);err!=nil{t.Fatal(err)}
        if delivered {break}; if time.Now().After(before){t.Fatal("reset email was not delivered")};time.Sleep(5*time.Millisecond)
    }
    code:=sink.latest()
    if len(code)!=6 {t.Fatal("missing reset code")}
    verifyMail:=func(ticket,value string,want int)*httptest.ResponseRecorder {
        return resetMailRequest(t,router,ticket,"confirm",map[string]string{"method":"email_code","username":"verification-user","browser_uuid":uuid,"verification_id":mail.VerificationID,"verification_code":value},want)
    }
    verifyMail(login("changed").MFATicket,code,400)
    wrong:="000000";if code==wrong{wrong="000001"}
    verifyMail(mailTicket,wrong,400)
    if deletion(rebound.AccessToken)==401 {t.Fatal("requesting a code or submitting an invalid code revoked the old session")}
    confirmed:=verifyMail(mailTicket,code,200)
    var resetByEmail auth.LoginResult
    if json.Unmarshal(confirmed.Body.Bytes(),&resetByEmail)!=nil || confirmed.Header().Get("X-MFA-Enrollment-Required")!="" {t.Fatal("missing enrollment restriction")}
    verifyMail(mailTicket,code,400)
    if deletion(rebound.AccessToken)!=401 || deletion(resetByEmail.AccessToken)!=403 {t.Fatal("email reset session boundaries")}
    emailCtx:=authenticate(resetByEmail.AccessToken,"/totp/setup")
    emailSetup,err:=m.BeginTOTPEnrollment(emailCtx,"");if err!=nil {t.Fatal(err)}
    rebound,err=m.ConfirmTOTPEnrollment(emailCtx,integrationTOTP(t,emailSetup.Secret));if err!=nil {t.Fatal(err)}
    checkNotice("totp_bound",2);checkNotice("totp_rebound",1)
    // Signed-in reset must verify this session's own reset proof before rotating credentials.
    accountReset := func(path, token, recovery string, body map[string]string, want int) *httptest.ResponseRecorder {
        t.Helper()
        payload, err := json.Marshal(body); if err != nil { t.Fatal(err) }
        req := httptest.NewRequest("POST", "/totp/recovery/"+path, strings.NewReader(string(payload)))
        if token != "" {req.Header.Set("Authorization", "Bearer "+token)}
        req.Header.Set("Content-Type", "application/json")
        if recovery != "" { req.Header.Set(authn.RecoveryCodeHeader, recovery) }
        w := httptest.NewRecorder(); router.ServeHTTP(w, req)
        if w.Code != want { t.Fatalf("account recovery %s: got %d want %d", path, w.Code, want) }
        return w
    }
    rebindAccount := func(w *httptest.ResponseRecorder, oldToken string) *auth.TOTPEnrollment {
        t.Helper()
        var replacement auth.LoginResult
        if json.Unmarshal(w.Body.Bytes(), &replacement) != nil || w.Header().Get("X-MFA-Enrollment-Required") != "" { t.Fatal("missing restricted session") }
        if deletion(oldToken) != 401 || deletion(replacement.AccessToken) != 403 { t.Fatal("invalid session boundaries") }
        issued = append(issued, &replacement)
        setupCtx := authenticate(replacement.AccessToken, "/totp/setup")
        setup, err := m.BeginTOTPEnrollment(setupCtx, ""); if err != nil { t.Fatal(err) }
        bound, err := m.ConfirmTOTPEnrollment(setupCtx, integrationTOTP(t,setup.Secret)); if err != nil { t.Fatal(err) }
        issued = append(issued, bound.LoginResult)
        return bound
    }
    if _,err:=store.DB.ExecContext(t.Context(),`UPDATE t_auth_email_cooldown SET retry_at=0`);err!=nil {t.Fatal(err)}
    accountReset("email-code", "", "", map[string]string{"browser_uuid":uuid},401)
    sent = accountReset("email-code", rebound.AccessToken, "",map[string]string{"browser_uuid":uuid},200)
    if err:=json.Unmarshal(sent.Body.Bytes(), &mail); err!=nil || mail.VerificationID=="" {t.Fatal("missing account reset code")}
    before=time.Now().Add(3*time.Second)
    for {
        var delivered bool
        if err:=store.DB.QueryRowContext(t.Context(),`SELECT delivered FROM t_auth_email_code WHERE verification_id=$1`,mail.VerificationID).Scan(&delivered);err!=nil{t.Fatal(err)}
        if delivered {break}; if time.Now().After(before){t.Fatal("account reset email was not delivered")};time.Sleep(5*time.Millisecond)
    }
    proof:=map[string]string{"browser_uuid":uuid,"verification_id":mail.VerificationID,"verification_code":sink.latest()}
    badProof:=map[string]string{"browser_uuid":"255f92f6-4049-4322-8c1a-e0095558aecc","verification_id":mail.VerificationID,"verification_code":proof["verification_code"]}
    accountReset("confirm",rebound.AccessToken,"",badProof,400)
    rebound = rebindAccount(accountReset("confirm",rebound.AccessToken,"",proof,200),rebound.AccessToken)
    accountReset("confirm",rebound.AccessToken,"",proof,400)
    accountReset("confirm",rebound.AccessToken,"invalid",map[string]string{},400)
    rebound = rebindAccount(accountReset("confirm",rebound.AccessToken,rebound.RecoveryCodes[0],map[string]string{},200),rebound.AccessToken)
    ctx=authenticate(rebound.AccessToken,"/totp")
    refreshed=rebound.LoginResult
    if err:=m.ResetUserTOTP(ctx,101);err!=nil {t.Fatal(err)}
    if deletion(refreshed.AccessToken)!=401 {t.Fatal("administrator reset left session valid")}
    var count int
    if err:=store.DB.QueryRowContext(ctx,`SELECT (SELECT COUNT(*) FROM t_user_totp WHERE user_id=101)+(SELECT COUNT(*) FROM t_user_recovery_code WHERE user_id=101)`).Scan(&count);err!=nil || count!=0 {t.Fatal("reset left credentials",err)}
    if _,err:=m.VerifySensitiveTOTP(ctx,integrationTOTP(t,reboundSetup.Secret));!errors.Is(err,auth.ErrUnauthorized) {t.Fatal("old identity or code survived reset",err)}
}
func resetMailRequest(t *testing.T, router http.Handler, ticket, step string, payload map[string]string, want int)*httptest.ResponseRecorder {
    t.Helper()
    challengeRequest:=httptest.NewRequest("POST","/pub/challenge",strings.NewReader(`{"purpose":"login"}`))
    challengeRequest.Header.Set("Content-Type","application/json")
    res:=httptest.NewRecorder(); router.ServeHTTP(res,challengeRequest)
    var challenge struct { ChallengeID string `json:"challenge_id"`; PublicKey jose.JSONWebKey `json:"public_key"` }
    if res.Code!=200 || json.Unmarshal(res.Body.Bytes(),&challenge)!=nil {t.Fatal("public challenge",res.Code)}
    enc,err:=jose.NewEncrypter(jose.A256GCM,jose.Recipient{Algorithm:jose.RSA_OAEP_256,Key:challenge.PublicKey},(&jose.EncrypterOptions{}).WithType("login+jwe"))
    if err!=nil{t.Fatal(err)}
    payload["challenge_id"]=challenge.ChallengeID
    raw,err:=json.Marshal(payload);if err!=nil{t.Fatal(err)}
    message,err:=enc.Encrypt(raw);if err!=nil{t.Fatal(err)}
    compact,err:=message.CompactSerialize();if err!=nil{t.Fatal(err)}
    input,err:=json.Marshal(map[string]string{"challenge_id":challenge.ChallengeID,"credential":compact});if err!=nil{t.Fatal(err)}
    req:=httptest.NewRequest("POST","/pub/totp/reset/"+step,strings.NewReader(string(input)))
    req.Header.Set("Content-Type","application/json");req.Header.Set(authn.MFATicketHeader,ticket)
    res=httptest.NewRecorder();router.ServeHTTP(res,req)
    if res.Code!=want {t.Fatalf("reset %s: status %d, want %d",step,res.Code,want)}
    return res
}

func setSensitiveProof(t *testing.T, router http.Handler, r *http.Request, token string, payload map[string]string) {
    t.Helper()
    challengeRequest:=httptest.NewRequest("POST","/challenge",strings.NewReader(`{"purpose":"sensitive"}`))
    challengeRequest.Header.Set("Authorization", "Bearer "+token)
    challengeRequest.Header.Set("Content-Type","application/json")
    out:=httptest.NewRecorder()
    router.ServeHTTP(out,challengeRequest)
    var challenge struct { ChallengeID string `json:"challenge_id"`; PublicKey jose.JSONWebKey `json:"public_key"` }
    if out.Code!=200 || json.Unmarshal(out.Body.Bytes(),&challenge)!=nil { t.Fatal("sensitive challenge",out.Code) }
    enc,err:=jose.NewEncrypter(jose.A256GCM,jose.Recipient{Algorithm:jose.RSA_OAEP_256,Key:challenge.PublicKey},(&jose.EncrypterOptions{}).WithType("sensitive+jwe"))
    if err!=nil {t.Fatal(err)}
    payload["challenge_id"]=challenge.ChallengeID
    raw,err:=json.Marshal(payload); if err!=nil {t.Fatal(err)}
    encrypted,err:=enc.Encrypt(raw);if err!=nil {t.Fatal(err)}
    compact,err:=encrypted.CompactSerialize();if err!=nil {t.Fatal(err)}
    r.Header.Set(authn.StepUpChallengeHeader,challenge.ChallengeID)
    r.Header.Set(authn.StepUpCredentialHeader,compact)
}

func TestEmailOnlySensitiveOperation(t *testing.T) {
    store:=permissionDatabase(t)
    state:=auth.NewRedisVerificationStore(sessionRedisClient(t))
    key:=strings.Repeat("s",64)
    memory:=auth.NewMemorySessionStore()
    t.Cleanup(func(){ _=memory.Close() })
    sessions,err:=auth.NewSessions(memory,auth.SessionConfig{Issuer:"step",Key:key,SessionLifetime:time.Hour,RememberLifetime:time.Hour,RetryWindow:time.Second}); if err!=nil {t.Fatal(err)}
    manager,err:=authn.New("step",key,time.Hour,auth.NewIdentityValidator(store,sessions)); if err!=nil {t.Fatal(err)}
    verification,err:=auth.NewVerification(store,state,auth.VerificationConfig{EmailEnabled:true,Issuer:"step",SigningKey:key,LoginTicketTTL:time.Minute,TOTPPeriod:time.Second,TOTPEnrollmentTTL:time.Minute,SensitiveLifetime:time.Hour,RateLimitWindow:time.Minute,RateLimitMaxAttempts:10}); if err!=nil {t.Fatal(err)}
    credentials,err:=auth.NewCredentials("step","",time.Minute,auth.NewMemoryChallengeStore()); if err!=nil {t.Fatal(err)}
    m:=auth.New(store,manager,credentials,sessions,nil,nil,nil,testLocalization(t,store),nil,verification)
    sink:=&emailSink{}
    codes,err:=newTemplateEmailCodes(store,sink,auth.EmailConfig{CodeTTL:time.Minute,ResendInterval:time.Minute,SendLimit:3,MaxAttempts:3,AlertTiers:[]auth.AlertTier{ {Window:time.Minute,Count:3} }},key); if err!=nil {t.Fatal(err)}
    m.SetEmailCodes(codes)

    stopWorkers, err := codes.StartWorkers(t.Context())
    if err != nil { t.Fatal(err) }
    t.Cleanup(stopWorkers)
    manager.SetHTTPSecurityPolicy(m.CheckHTTPSecurity)
    probe:=&verificationProbe{}
    router,err:=server.NewRouter(&config.Config{},manager,nil,m,probe);if err!=nil {t.Fatal(err)}
    _,err=store.DB.ExecContext(t.Context(),`INSERT INTO t_user(id,username,code,password,status,login_count,email,email_verified_at) VALUES(101,'email-user','EMAIL101','unused',1,1,'owner@example.test',1)`);if err!=nil {t.Fatal(err)}
    login:=func() string {
        identity:=authn.Identity{Methods: authn.Methods{Primary: authn.Password}, UserID:101,Code:"EMAIL101",Username:"email-user",CredentialVersion:1}
        session,err:=sessions.Issue(t.Context(),identity,false);if err!=nil {t.Fatal(err)}
        identity.SessionID=session.SessionID
        token,_,err:=manager.Issue(identity);if err!=nil {t.Fatal(err)}
        return token
    }
    token,other:=login(),login()
    request:=func(method,path,body,session string)*http.Request{
        r:=httptest.NewRequest(method,path,strings.NewReader(body))
        r.Header.Set("Authorization","Bearer "+session)
        r.Header.Set("Content-Type","application/json")
        return r
    }
    run:=func(r *http.Request,want int)*httptest.ResponseRecorder{
        t.Helper()
        w:=httptest.NewRecorder();router.ServeHTTP(w,r)
        if w.Code!=want {t.Fatalf("%s %s: got %d, want %d: %s",r.Method,r.URL.Path,w.Code,want,w.Body.String())}
        return w
    }
    const target="/totp-probe/1"
    const uuid="c6eefc2b-1e8d-4446-9f69-3f2bd62d8a75"
    const body=`{"browser_uuid":"`+uuid+`"}`
    run(request("DELETE",target,"",token),403)
    run(request("POST","/email/sensitive-code",`{"browser_uuid":"`+uuid+`","email":"elsewhere@example.test"}`,token),400)
    queued:=run(request("POST","/email/sensitive-code",body,token),200)
    var ticket auth.EmailCodeStatus
    if json.Unmarshal(queued.Body.Bytes(),&ticket)!=nil || ticket.VerificationID=="" {t.Fatal("missing ticket")}
    run(request("POST","/email/sensitive-code",body,token),429)
    deadline:=time.Now().Add(5*time.Second)
    for {
        var delivered bool
        if err:=store.DB.QueryRowContext(t.Context(),`SELECT delivered FROM t_auth_email_code WHERE verification_id=$1`,ticket.VerificationID).Scan(&delivered);err!=nil {t.Fatal(err)}
        if delivered {break}
        if time.Now().After(deadline) {t.Fatal("email not delivered")}
        time.Sleep(5*time.Millisecond)
    }
    verify:=func(session,challengeSession,code string)*http.Request{
        r:=request("DELETE",target,"",session)
        setSensitiveProof(t,router,r,challengeSession,map[string]string{"method":"email","verification_code":code,"browser_uuid":uuid,"verification_id":ticket.VerificationID})
        return r
    }
    code:=sink.latest()
    // Simulate elapsed send cooldown without expiring the first session's code.
    if _,err:=store.DB.ExecContext(t.Context(),`UPDATE t_auth_email_cooldown SET retry_at=0`);err!=nil {t.Fatal(err)}
    run(request("POST","/email/sensitive-code",body,other),200)
    wrong:="000000"; if code==wrong {wrong="000001"}
    run(verify(token,token,wrong),400)
    run(verify(other,token,code),400)
    run(verify(other,other,code),400)
    if probe.mutations.Load()!=0 {t.Fatal("unverified operation executed")}
    run(verify(token,token,code),204)
    run(request("DELETE",target,"",token),204)
    // Exercise a real mutation after the session has a grant, including a rolled-back write.
    limits,_:=pagination.New(100,100)
    dict,err:=dictionary.New(store,dictionary.NewMemoryValueCache(),dictionary.CacheTTL,limits,testLocalization(t,store));if err!=nil {t.Fatal(err)}
    secured,err:=manager.AuthenticateRequest(request("POST","/dictionary","",token));if err!=nil {t.Fatal(err)}
    if err:=manager.CheckHTTPSecurity(secured);err!=nil {t.Fatal(err)}
    input:=dictionary.CreateDictionaryInput{Key:"TEST_SENSITIVE_MAIL",Name:"Fixture",Value:json.RawMessage(`{"test":true}`)}
    if _,err:=dict.Create(secured.Context(),input);err!=nil {t.Fatal(err)}
    if _,err:=dict.Create(secured.Context(),input);err==nil {t.Fatal("expected duplicate write to roll back")}
    var recorded int
    if err:=store.DB.QueryRowContext(t.Context(),`SELECT count(*) FROM t_sensitive_operation WHERE user_id=101 AND operation_kind='create' AND NOT permission_related`).Scan(&recorded);err!=nil || recorded!=1 {t.Fatal("committed sensitive events",recorded,err)}
    run(request("DELETE",target,"",other),403)
    if probe.mutations.Load()!=2 {t.Fatal("session grant was not isolated")}
}
{{ end -}}
