package tests

import (
	"errors"
	"os"
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

func TestAuditMessageUpgradePreservesCustomTranslations(t *testing.T) {
	store:=permissionDatabase(t)
	const restoreOld=`UPDATE t_dictionary SET value=jsonb_set(jsonb_set(value,'{messages,en-US,backend,AUDIT_EXPORT_LIMIT}',to_jsonb('More than 10,000 events. Narrow the filters before exporting.'::text)),'{messages,zh-CN,backend,AUDIT_FILTER_INVALID}',to_jsonb('自定义错误提示'::text)) WHERE dictionary_key='I18N_CATALOG'`
	if _,err:=store.SQL(t.Context()).ExecContext(t.Context(),restoreOld);err!=nil {t.Fatal(err)}
	script,err:=os.ReadFile("../../scripts/upgrade-audit-limit-messages.sql");if err!=nil {t.Fatal(err)}
	for range 2 { if _,err:=store.SQL(t.Context()).ExecContext(t.Context(),string(script));err!=nil {t.Fatal(err)} }
	var english,chinese string
	if err:=store.SQL(t.Context()).QueryRowContext(t.Context(),`SELECT value #>> '{messages,en-US,backend,AUDIT_EXPORT_LIMIT}',value #>> '{messages,zh-CN,backend,AUDIT_FILTER_INVALID}' FROM t_dictionary WHERE dictionary_key='I18N_CATALOG'`).Scan(&english,&chinese);err!=nil {t.Fatal(err)}
	if english!="The export exceeds the configured row limit. Narrow the filters before exporting." || chinese!="自定义错误提示" {t.Fatal("upgrade did not preserve custom messages",english,chinese)}
}
