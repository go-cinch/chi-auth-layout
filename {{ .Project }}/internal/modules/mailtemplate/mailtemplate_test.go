package mailtemplate

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "reflect"
 "strings"
 "testing"
 "github.com/DATA-DOG/go-sqlmock"
 "github.com/lib/pq"
 "{{ .Computed.module_name_final }}/internal/common/pagination"
 "{{ .Computed.module_name_final }}/internal/infra/db"
)

func testModule(t *testing.T) (*Module,sqlmock.Sqlmock) {
 t.Helper()
 database,mock,err:=sqlmock.New();if err!=nil { t.Fatal(err) }
 t.Cleanup(func(){if err:=mock.ExpectationsWereMet();err!=nil {t.Error(err)};_ = database.Close()})
 limits,_:=pagination.New(100,100)
 return New(&db.Store{DB:database},limits),mock
}

func sampleContent() Content {
 return Content{"zh-CN":{Subject:"验证码",HTML:"<p>\x7b\x7bcode\x7d\x7d</p>",Text:"code",PreviewValues:map[string]string{"code":"123456"}},"en-US":{Subject:"Code",HTML:"<p>\x7b\x7bcode\x7d\x7d</p>",Text:"Code",PreviewValues:map[string]string{}}}
}

func recordRow(content bool,revision,version int64,draft bool, templateIDs ...string) *sqlmock.Rows {
 templateID:="notice.test";if len(templateIDs)>0{templateID=templateIDs[0]}
 columns:=strings.Split(strings.ReplaceAll(recordColumns," ",""),",")
 if content { columns=append(columns,"content") }
 rows:=sqlmock.NewRows(columns)
 if content { raw,_:=json.Marshal(sampleContent());return rows.AddRow(1,100,200,templateID,"Test","notification",revision,version,draft,raw) }
 return rows.AddRow(1,100,200,templateID,"Test","notification",revision,version,draft)
}

func TestParametersAndValidation(t *testing.T) {
 if got:=Parameters("\x7b\x7b code \x7d\x7d \x7b\x7bcode\x7d\x7d \x7b\x7baccount\x7d\x7d");!reflect.DeepEqual(got,[]string{"code","account"}) {t.Fatal(got)}
 if err:=validate("Test","verification",sampleContent(),true);err!=nil {t.Fatal(err)}
 for _,category:=range []string{"confirmation","security","notification"} {if !validCategory(category){t.Fatal(category)}}
 for _,edit:=range []func(Content){
  func(c Content){delete(c,"zh-CN")},
  func(c Content){c["other"]=c["en-US"];delete(c,"zh-CN")},
  func(c Content){v:=c["zh-CN"];v.Subject="";c["zh-CN"]=v},
  func(c Content){v:=c["zh-CN"];v.Subject="a\nb";c["zh-CN"]=v},
  func(c Content){v:=c["zh-CN"];v.HTML="";c["zh-CN"]=v},
  func(c Content){v:=c["zh-CN"];v.HTML="\x7b\x7bbad()\x7d\x7d";c["zh-CN"]=v},
  func(c Content){v:=c["zh-CN"];v.PreviewValues["bad name"]="x";c["zh-CN"]=v},
  func(c Content){v:=c["zh-CN"];v.PreviewValues["code"]=strings.Repeat("a",10001);c["zh-CN"]=v},
 } {c:=sampleContent();edit(c);if !errors.Is(validate("Test","notification",c,true),ErrInvalid){t.Fatal("accepted invalid content")}}
 for _,name:=range []string{"",strings.Repeat("中",101)} {if validate(name,"notification",sampleContent(),false)==nil {t.Fatal(name)}}
 if validate("name","unknown",sampleContent(),false)==nil {t.Fatal("invalid category")}
 c:=sampleContent();v:=c["zh-CN"];v.HTML="";v.Subject="";c["zh-CN"]=v;if err:=validate("Draft","notification",c,false);err!=nil {t.Fatal(err)}
}

func TestCreateGetAndList(t *testing.T) {
 m,mock:=testModule(t);ctx:=context.Background()
 input:=CreateInput{TemplateID:"notice.test",Name:" Test ",Category:"notification",Content:sampleContent()}
 mock.ExpectBegin();mock.ExpectQuery("INSERT INTO t_mail_template").WithArgs("notice.test","Test","notification",sqlmock.AnyArg()).WillReturnRows(recordRow(true,1,0,true));mock.ExpectCommit()
 if r,err:=m.Create(ctx,input);err!=nil||r.ID!=1 {t.Fatalf("create: %+v %v",r,err)}
 mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WithArgs(int64(1)).WillReturnRows(recordRow(true,1,0,true))
 if r,err:=m.Get(ctx,1);err!=nil||r.Content["zh-CN"].Subject!="验证码" {t.Fatalf("get: %+v %v",r,err)}
 mock.ExpectQuery("SELECT count").WithArgs("test","").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
 mock.ExpectQuery("SELECT id, created_at").WithArgs("test","",int32(1),int64(0)).WillReturnRows(recordRow(false,1,0,true))
 if page,err:=m.List(ctx,ListInput{Q:"test"});err!=nil||page.T!=1||len(page.Items)!=1||page.Items[0].Content!=nil {t.Fatalf("list: %+v %v",page,err)}
 out:=int32(101);if page,err:=m.List(ctx,ListInput{P:&out});err!=nil||page.T!=0 {t.Fatal(page,err)}
 if _,err:=m.List(ctx,ListInput{Category:"bad"});!errors.Is(err,ErrInvalid){t.Fatal(err)}
 for _,id:=range []int64{-1,0} {if _,err:=m.Get(ctx,id);!errors.Is(err,ErrInvalid){t.Fatal(err)};if err:=m.Delete(ctx,id);!errors.Is(err,ErrInvalid){t.Fatal(err)}}
 input.TemplateID="INVALID";if _,err:=m.Create(ctx,input);!errors.Is(err,ErrInvalid){t.Fatal(err)}
 input.TemplateID="valid";input.Name="";if _,err:=m.Create(ctx,input);!errors.Is(err,ErrInvalid){t.Fatal(err)}
}

func TestDraftPublicationAndHistory(t *testing.T) {
 m,mock:=testModule(t);ctx:=context.Background();name,category,content:="Next","security",sampleContent()
 mock.ExpectBegin();mock.ExpectQuery("SELECT .* FOR UPDATE").WithArgs(int64(1)).WillReturnRows(recordRow(true,2,1,false));mock.ExpectQuery("UPDATE t_mail_template SET name").WithArgs(int64(1),name,category,sqlmock.AnyArg(),"notice.test").WillReturnRows(recordRow(true,3,1,true));mock.ExpectCommit()
 r,err:=m.Update(ctx,1,UpdateInput{Revision:2,Name:&name,Category:&category,Content:&content});if err!=nil||r.Revision!=3||r.PublishedVersion!=1{t.Fatal(r,err)}
 mock.ExpectBegin();mock.ExpectQuery("SELECT .* FOR UPDATE").WithArgs(int64(1)).WillReturnRows(recordRow(true,3,1,true));mock.ExpectExec("INSERT INTO t_mail_template_version").WillReturnResult(sqlmock.NewResult(10,1));mock.ExpectQuery("UPDATE t_mail_template SET published_version").WillReturnRows(recordRow(true,4,2,false));mock.ExpectCommit()
 r,err=m.Publish(ctx,1,PublishInput{Revision:3});if err!=nil||r.PublishedVersion!=2||r.HasDraft{t.Fatal(r,err)}
 mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true));mock.ExpectQuery("SELECT id,created_at,version").WillReturnRows(sqlmock.NewRows([]string{"id","created_at","version","name","category","published_by"}).AddRow(10,200,2,"Test","notification",0))
 if versions,err:=m.Versions(ctx,1);err!=nil||len(versions)!=1{t.Fatal(versions,err)}
 raw,_:=json.Marshal(content);mock.ExpectQuery("SELECT id,created_at,version").WithArgs(int64(1),int64(2)).WillReturnRows(sqlmock.NewRows([]string{"id","created_at","version","name","category","published_by","content"}).AddRow(10,200,2,"Test","notification",0,raw))
 if v,err:=m.GetVersion(ctx,1,2);err!=nil||v.Content==nil{t.Fatal(v,err)}
 mock.ExpectBegin();mock.ExpectExec("DELETE FROM t_mail_template").WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0,1));mock.ExpectCommit();if err:=m.Delete(ctx,1);err!=nil {t.Fatal(err)}
}

func TestConflictsRollbackAndDatabaseFailures(t *testing.T) {
 m,mock:=testModule(t);ctx:=context.Background();name:="Changed"
 for _,op:=range []string{"update","publish"} {
  mock.ExpectBegin();mock.ExpectQuery("SELECT .* FOR UPDATE").WillReturnRows(recordRow(true,9,1,true));mock.ExpectRollback()
  var err error;if op=="update"{_,err=m.Update(ctx,1,UpdateInput{Revision:8,Name:&name})}else{_,err=m.Publish(ctx,1,PublishInput{Revision:8})};if !errors.Is(err,ErrConflict){t.Fatal(err)}
 }
 mock.ExpectBegin();mock.ExpectQuery("SELECT .* FOR UPDATE").WillReturnRows(recordRow(true,9,1,false));mock.ExpectRollback();if _,err:=m.Publish(ctx,1,PublishInput{Revision:9});!errors.Is(err,ErrConflict){t.Fatal(err)}
 mock.ExpectBegin();mock.ExpectQuery("INSERT INTO t_mail_template").WillReturnError(&pq.Error{Code:"23505"});mock.ExpectRollback();if _,err:=m.Create(ctx,CreateInput{TemplateID:"a",Name:"Name",Category:"notification",Content:sampleContent()});!errors.Is(err,ErrDuplicate){t.Fatal(err)}
 mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WillReturnError(sql.ErrNoRows);if _,err:=m.Get(ctx,1);!errors.Is(err,ErrNotFound){t.Fatal(err)}
 mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WillReturnError(sql.ErrConnDone);if _,err:=m.Get(ctx,1);!errors.Is(err,sql.ErrConnDone){t.Fatal(err)}
 mock.ExpectQuery("SELECT count").WillReturnError(sql.ErrConnDone);if _,err:=m.List(ctx,ListInput{});err==nil{t.Fatal("count error")}
 mock.ExpectQuery("SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1));mock.ExpectQuery("SELECT id, created_at").WillReturnError(sql.ErrConnDone);if _,err:=m.List(ctx,ListInput{});err==nil{t.Fatal("list error")}
 for _,input:=range []UpdateInput{ {},{Revision:1}} {if _,err:=m.Update(ctx,1,input);!errors.Is(err,ErrInvalid){t.Fatal(err)}}
 if _,err:=m.Publish(ctx,0,PublishInput{});!errors.Is(err,ErrInvalid){t.Fatal(err)}
 if _,err:=m.Versions(ctx,0);!errors.Is(err,ErrInvalid){t.Fatal(err)}
 if _,err:=m.GetVersion(ctx,0,1);!errors.Is(err,ErrInvalid){t.Fatal(err)}
 mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false));if _,err:=m.Versions(ctx,9);!errors.Is(err,ErrNotFound){t.Fatal(err)}
 mock.ExpectQuery("SELECT id,created_at,version").WillReturnError(sql.ErrNoRows);if _,err:=m.GetVersion(ctx,1,4);!errors.Is(err,ErrNotFound){t.Fatal(err)}
 mock.ExpectBegin();mock.ExpectExec("DELETE FROM t_mail_template").WillReturnResult(sqlmock.NewResult(0,0));mock.ExpectRollback();if err:=m.Delete(ctx,1);!errors.Is(err,ErrNotFound){t.Fatal(err)}
}

func TestPreviewExamplesAreOptionalButReturnObjects(t *testing.T) {
 content:=sampleContent();value:=content["en-US"];value.PreviewValues=nil;content["en-US"]=value
 normalizePreviewValues(content)
 if content["en-US"].PreviewValues==nil || content["zh-CN"].PreviewValues["code"]!="123456" {t.Fatal("preview values not normalized")}
}

func TestTemplateIDCanBeRenamedWithoutChangingRecordIdentity(t *testing.T) {
 m,mock:=testModule(t);ctx:=context.Background();nextID:="  order.payment_success  "
 mock.ExpectBegin();mock.ExpectQuery("SELECT .* FOR UPDATE").WithArgs(int64(1)).WillReturnRows(recordRow(true,3,2,false))
 mock.ExpectQuery("UPDATE t_mail_template SET name").WithArgs(int64(1),"Test","notification",sqlmock.AnyArg(),"order.payment_success").WillReturnRows(recordRow(true,4,2,true,"order.payment_success"));mock.ExpectCommit()
 value,err:=m.Update(ctx,1,UpdateInput{Revision:3,TemplateID:&nextID})
 if err!=nil||value.ID!=1||value.TemplateID!="order.payment_success"||value.PublishedVersion!=2 {t.Fatal(value,err)}
 for _,invalid:=range []string{"", "UpperCase", "has space",strings.Repeat("a",101)} {if _,err:=m.Update(ctx,1,UpdateInput{Revision:4,TemplateID:&invalid});!errors.Is(err,ErrInvalid){t.Fatal("accepted invalid ID",invalid,err)}}
 mock.ExpectBegin();mock.ExpectQuery("SELECT .* FOR UPDATE").WillReturnRows(recordRow(true,4,2,true))
 mock.ExpectQuery("UPDATE t_mail_template SET name").WillReturnError(&pq.Error{Code:"23505",Constraint:"uk_mail_template_template_id"});mock.ExpectRollback()
 if _,err:=m.Update(ctx,1,UpdateInput{Revision:4,TemplateID:&nextID});!errors.Is(err,ErrDuplicate){t.Fatal("duplicate mapping",err)}
}

func TestDiscardDraft(t *testing.T) {
	yes, no := true, false
	name := "Other"
	invalidFlag := UpdateInput{Revision: 1, DiscardDraft: &no}
	mixed := UpdateInput{Revision: 1, DiscardDraft: &yes, Name: &name}
	for _, input := range []UpdateInput{invalidFlag, mixed} {
		m, _ := testModule(t)
		if _, err := m.Update(t.Context(), 1, input); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name               string
		revision, version  int64
		failure, errorWant error
	}{
		{"restore", 2, 1, nil, nil},
		{"unpublished", 2, 0, nil, ErrInvalid},
		{"conflict", 3, 1, nil, ErrConflict},
		{"version failure", 2, 1, sql.ErrConnDone, sql.ErrConnDone},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, mock := testModule(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .* FOR UPDATE").WithArgs(int64(1)).WillReturnRows(recordRow(true, test.revision, test.version, true))
			if test.revision == 2 && test.version > 0 {
				query := mock.ExpectQuery("SELECT id,created_at,version").WithArgs(int64(1), int64(1))
				if test.failure != nil {
					query.WillReturnError(test.failure)
				} else {
					raw, _ := json.Marshal(sampleContent())
					query.WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "version", "name", "category", "published_by", "content"}).AddRow(10, 200, 1, "Published", "security", 0, raw))
					mock.ExpectQuery("UPDATE t_mail_template SET name=.*has_draft=FALSE").WithArgs(int64(1), "Published", "security", sqlmock.AnyArg()).WillReturnRows(recordRow(true, 3, 1, false))
				}
			}
			if test.errorWant == nil {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			result, err := m.Update(t.Context(), 1, UpdateInput{Revision: 2, DiscardDraft: &yes})
			if !errors.Is(err, test.errorWant) {
				t.Fatal(err)
			}
			if err == nil && (result.HasDraft || result.PublishedVersion != 1 || result.Revision != 3) {
				t.Fatal(result)
			}
		})
	}
}
