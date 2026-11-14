package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
	"{{ .Computed.module_name_final }}/internal/modules/msg"
)

func TestRunStopsMessageExpiryWorker(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("SELECT user_id FROM t_user_msg_status.*LIMIT 100").WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	limits, _ := pagination.New(100, 100)
	application := &Application{server: &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()}, msgModule: msg.New(&db.Store{DB: database}, limits)}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for mock.ExpectationsWereMet() != nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("expiry worker did not stop")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunLocalizationRefreshInterval(t *testing.T) {
	for _, interval := range []time.Duration{0, 10 * time.Millisecond} {
		t.Run(interval.String(), func(t *testing.T) {
			queried := make(chan struct{}, 2)
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(expected, actual string) error {
				if err := sqlmock.QueryMatcherRegexp.Match(expected, actual); err != nil {
					return err
				}
				queried <- struct{}{}
				return nil
			})))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			limits, err := pagination.New(10000, 10000)
			if err != nil {
				t.Fatal(err)
			}
			module, err := dictionary.New(&db.Store{DB: database}, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, localization.New())
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(dictionary.LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"value"}))
			if err := module.LoadLocalization(t.Context()); err != nil {
				t.Fatal(err)
			}
			<-queried
			mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(dictionary.LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"value"}))
			application := &Application{
				server:                      &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()},
				dictionaryModule:            module,
				localizationRefreshInterval: interval,
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- application.Run(ctx) }()
			if interval > 0 {
				select {
				case <-queried:
				case <-time.After(2 * time.Second):
					cancel()
					<-done
					t.Fatal("configured interval did not trigger a refresh")
				}
			} else {
				select {
				case <-queried:
					t.Error("zero interval must disable polling")
				case <-time.After(30 * time.Millisecond):
				}
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if interval > 0 {
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
