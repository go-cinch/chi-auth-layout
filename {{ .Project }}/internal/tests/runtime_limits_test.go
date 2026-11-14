package tests

import (
	"errors"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/audit"
)

func TestConfiguredAuditLimitsPostgres(t *testing.T) {
	store:=permissionDatabase(t)
	limits,_:=pagination.New(100,100)
	settings:=config.DefaultAudit();settings.DefaultWindow=time.Hour;settings.MaxWindow=2*time.Hour;settings.ExportMaxRows=2
	m:=audit.New(store,limits,settings)
	for i:=0;i<3;i++ {
		if err:=m.RecordHTTP(t.Context(),auditctx.Event{Kind:"login",Status:200,OccurredAt:time.Now().Add(-time.Minute).UnixMilli(),Username:"config-test"});err!=nil {t.Fatal(err)}
	}
	if _,err:=m.Export(t.Context(),audit.Filter{Kind:"login"});!errors.Is(err,audit.ErrExportLimit) {t.Fatal("PostgreSQL export ignored configured row limit",err)}
	settings.ExportMaxRows=3
	m=audit.New(store,limits,settings)
	if rows,err:=m.Export(t.Context(),audit.Filter{Kind:"login"});err!=nil || len(rows)!=3 {t.Fatal("PostgreSQL export boundary failed",err)}
	from,to:=time.Now().Add(-3*time.Hour).UnixMilli(),time.Now().UnixMilli()
	if _,err:=m.List(t.Context(),audit.Filter{Kind:"login",From:&from,To:&to});!errors.Is(err,audit.ErrFilter) {t.Fatal("configured maximum range ignored",err)}
}
