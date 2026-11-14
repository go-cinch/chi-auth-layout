package db

import "context"

type mutationRecorderKey struct{}

type mutationRecorder struct {
	prepare func(context.Context) context.Context
	record  func(context.Context) error
}

// WithMutationRecorder initializes per-transaction state and records only successful
// mutations, within the same transaction. Preliminary reads continue to use Tx.
func WithMutationRecorder(ctx context.Context, prepare func(context.Context) context.Context, record func(context.Context) error) context.Context {
	return context.WithValue(ctx, mutationRecorderKey{}, mutationRecorder{prepare: prepare, record: record})
}

func HasMutationRecorder(ctx context.Context) bool {
	return ctx.Value(mutationRecorderKey{}) != nil
}

func (s *Store) MutationTx(ctx context.Context, change func(context.Context) error) error {
	recorder, ok := ctx.Value(mutationRecorderKey{}).(mutationRecorder)
	if !ok {
		return s.Tx(ctx, change)
	}
	return s.Tx(ctx, func(tx context.Context) error {
		if recorder.prepare != nil {
			tx = recorder.prepare(tx)
		}
		if err := change(tx); err != nil {
			return err
		}
		return recorder.record(tx)
	})
}
