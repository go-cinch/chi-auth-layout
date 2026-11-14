package tests

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/mailtemplate"
)

func TestMailTemplateDraftPublicationPostgres(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	module := mailtemplate.New(store, limits)
	content := mailtemplate.Content{"zh-CN": {Subject: "通知", HTML: "<p>\x7b\x7bcode\x7d\x7d</p>", Text: "通知", PreviewValues: map[string]string{"code": "123456"}}, "en-US": {Subject: "Notice", HTML: "<p>\x7b\x7bcode\x7d\x7d</p>", Text: "Notice"}}
	r, err := module.Create(t.Context(), mailtemplate.CreateInput{TemplateID: "test.notice", Name: "Notice", Category: "notification", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.Create(t.Context(), mailtemplate.CreateInput{TemplateID: "test.notice", Name: "Duplicate", Category: "notification", Content: content}); !errors.Is(err, mailtemplate.ErrDuplicate) {
		t.Fatal(err)
	}
	published, err := module.Publish(t.Context(), r.ID, mailtemplate.PublishInput{Revision: r.Revision})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			name := "Edited"
			_, err := module.Update(t.Context(), r.ID, mailtemplate.UpdateInput{Revision: published.Revision, Name: &name})
			results <- err
		}()
	}
	close(start)
	group.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, mailtemplate.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("writers: %d successes, %d conflicts", success, conflicts)
	}
	version, err := module.GetVersion(t.Context(), r.ID, 1)
	if err != nil || version.Name != "Notice" {
		t.Fatal(version, err)
	}
	current, err := module.Get(t.Context(), r.ID)
	if err != nil || !current.HasDraft || current.PublishedVersion != 1 {
		t.Fatal(current, err)
	}
	discard := true
	restored, err := module.Update(t.Context(), r.ID, mailtemplate.UpdateInput{Revision: current.Revision, DiscardDraft: &discard})
	if err != nil || restored.HasDraft || restored.PublishedVersion != 1 || restored.Revision != current.Revision+1 || restored.Name != version.Name || !reflect.DeepEqual(restored.Content, version.Content) {
		t.Fatalf("discard: %+v %v", restored, err)
	}
	history, err := module.Versions(t.Context(), r.ID)
	if err != nil || len(history) != 1 {
		t.Fatal("discard changed publication history", err)
	}
	if _, err = module.Update(t.Context(), r.ID, mailtemplate.UpdateInput{Revision: current.Revision, DiscardDraft: &discard}); !errors.Is(err, mailtemplate.ErrConflict) {
		t.Fatal("stale discard accepted", err)
	}
	if err := module.Delete(t.Context(), r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := module.GetVersion(t.Context(), r.ID, 1); !errors.Is(err, mailtemplate.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestMailTemplateMutationsRecordSuccessfulWrites(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	module := mailtemplate.New(store, limits)
	recorded := 0
	ctx := db.WithMutationRecorder(t.Context(), nil, func(tx context.Context) error {
		if store.SQL(tx) == store.DB {
			t.Fatal("recorder escaped write transaction")
		}
		recorded++
		return nil
	})
	content := mailtemplate.Content{"zh-CN": {Subject: "通知", HTML: "<p>测试</p>"}, "en-US": {Subject: "Notice", HTML: "<p>Test</p>"}}
	r, err := module.Create(ctx, mailtemplate.CreateInput{TemplateID: "test.recorded", Name: "Recorded", Category: "notification", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	name := "Updated"
	r, err = module.Update(ctx, r.ID, mailtemplate.UpdateInput{Revision: r.Revision, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	r, err = module.Publish(ctx, r.ID, mailtemplate.PublishInput{Revision: r.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = module.Update(ctx, r.ID, mailtemplate.UpdateInput{Revision: 1, Name: &name}); !errors.Is(err, mailtemplate.ErrConflict) {
		t.Fatal(err)
	}
	if err = module.Delete(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if recorded != 4 {
		t.Fatalf("recorded %d operations, want 4 successful writes", recorded)
	}
}
