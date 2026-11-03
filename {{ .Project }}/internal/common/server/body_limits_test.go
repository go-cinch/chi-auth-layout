package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/config"
{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{ else }}
	"github.com/go-chi/chi/v5"
{{ end }}
)

type bodyLimitModule struct { category BodyLimit }

func (*bodyLimitModule) Name() string { return "/body-probe" }

func (*bodyLimitModule) Public() bool { return true }

func (m *bodyLimitModule) handle(w http.ResponseWriter, r *http.Request) {
	var input struct { Name string `json:"name"` }
	if err := DecodeJSON(w,r,&input,m.category); err != nil { WriteError(w,r,400,err); return }
	WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" }}
func (m *bodyLimitModule) HTTP(r *gin.RouterGroup) { r.POST("/pub/body", func(c *gin.Context) {m.handle(c.Writer,c.Request)}) }
{{ else }}
func (m *bodyLimitModule) HTTP() http.Handler { r:=chi.NewRouter(); r.Post("/pub/body",m.handle); return r }
{{ end }}

func TestConfiguredBodyLimitsAndRouterIsolation(t *testing.T) {
	for _, category := range []BodyLimit{BodyAuthChallenge,BodySliderChallenge,BodyAuthVerification,BodyAuth,BodyBusiness,BodyDictionary} {
		t.Run(string(category),func(t *testing.T) {
			var routers []http.Handler
			for _, size := range []int{12,13} {
				cfg:=&config.Config{}
				cfg.HTTP.BodyLimits=config.HTTPBodyLimitsConfig{AuthChallenge:size,SliderChallenge:size,AuthVerification:size,Auth:size,Business:size,Dictionary:size}
				router,err:=NewRouter(cfg,nil,nil,&bodyLimitModule{category:category})
				if err!=nil {t.Fatal(err)}
				routers=append(routers,router)
			}
			for _, index := range []int{0,1,0} {
				w:=httptest.NewRecorder(); routers[index].ServeHTTP(w,httptest.NewRequest("POST","/body-probe/pub/body",strings.NewReader(`{"name":"ok"}`)))
				want:=200; if index==0 {want=400}
				if w.Code!=want {t.Fatalf("router %d = %d: %s",index,w.Code,w.Body.String())}
			}
		})
	}
	r:=httptest.NewRequest("POST","/",nil)
	if requestBodyLimit(r,BodyAuth)!=16384 || requestBodyLimit(r,BodyLimit("invalid"))!=0 {t.Fatal("standalone decoder defaults changed")}
	cfg:=&config.Config{};cfg.HTTP.BodyLimits.Auth=-1
	if _,err:=NewRouter(cfg,nil,nil);err==nil {t.Fatal("invalid body limit accepted")}
	cfg=&config.Config{};cfg.Audit.RecordTimeout=-1
	if _,err:=NewRouter(cfg,nil,nil);err==nil {t.Fatal("invalid audit policy accepted")}
}
