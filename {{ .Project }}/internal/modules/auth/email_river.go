package auth

import (
	"context"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
)

type emailDelivery struct {
	TicketID string `json:"ticket_id"`
}

func (emailDelivery) Kind() string { return "auth_email_deliver" }

type sensitiveActivityScan struct{}

func (sensitiveActivityScan) Kind() string { return "auth_sensitive_activity_scan" }

type historyCleanup struct{}

func (historyCleanup) Kind() string { return "auth_history_cleanup" }

type tenMinuteSlot struct{}

func (tenMinuteSlot) Next(now time.Time) time.Time {
	return now.Truncate(10 * time.Minute).Add(10 * time.Minute)
}

func (e *EmailCodes) StartWorkers(ctx context.Context) (func(), error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, river.WorkFunc(func(ctx context.Context, _ *river.Job[historyCleanup]) error {
		return e.CleanupHistory(ctx)
	}))
	river.AddWorker(workers, river.WorkFunc(e.sendTestEmail))
	river.AddWorker(workers, river.WorkFunc(func(ctx context.Context, job *river.Job[emailDelivery]) error {
		return e.handleEmail(ctx, job.Args.TicketID)
	}))
	river.AddWorker(workers, river.WorkFunc(func(ctx context.Context, _ *river.Job[sensitiveActivityScan]) error {
		return e.ScanSensitiveOperations(ctx)
	}))
	cleanup := river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour), func() (river.JobArgs, *river.InsertOpts) {
		return historyCleanup{}, &river.InsertOpts{Queue: "auth_history_cleanup", UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
	}, &river.PeriodicJobOpts{RunOnStart: true})
	periodic := []*river.PeriodicJob{cleanup}
	queues := map[string]river.QueueConfig{"auth_history_cleanup": {MaxWorkers: 1}}
	// Retention runs even with SMTP disabled. Delivery and security scans retain
	// their previous readiness gate so disabled transport cannot consume tickets.
	if e.sender.Ready() {
		queues["mail_template_test"] = river.QueueConfig{MaxWorkers: 1}
		queues["auth_email_interactive"] = river.QueueConfig{MaxWorkers: 2}
		queues["auth_email_notice"] = river.QueueConfig{MaxWorkers: 1}
		queues["email_sensitive_activity_notification"] = river.QueueConfig{MaxWorkers: 1}
		scan := river.NewPeriodicJob(tenMinuteSlot{}, func() (river.JobArgs, *river.InsertOpts) {
			return sensitiveActivityScan{}, &river.InsertOpts{Queue: "email_sensitive_activity_notification"}
		}, &river.PeriodicJobOpts{RunOnStart: true})
		periodic = append(periodic, scan)
	}

	client, err := river.NewClient(riverdatabasesql.New(e.store.DB), &river.Config{
		Workers:              workers,
		Queues:               queues,
		PeriodicJobs:         periodic,
		RescueStuckJobsAfter: 90 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	if err := client.Start(ctx); err != nil {
		return nil, err
	}
	return func() { _ = client.StopAndCancel(context.WithoutCancel(ctx)) }, nil
}
