# Database Guidelines

## Boundaries

- Keep database connection setup, optional database creation, pool configuration, transactions, and embedded migrations in this package.
- Keep business SQL and domain queries inside their owning capability packages.
- Expose small database abstractions only when they are used by multiple capabilities; prefer `*sql.DB`, `*sql.Tx`, and the existing `SQLExecutor` interface.
- Propagate `context.Context` through every database operation.

## Connection Lifecycle

- Validate the driver and DSN before opening a connection.
- Use `database.autoCreate` only when the configured account is allowed to create databases. Production deployments should normally provision databases outside the application.
- Configure pool limits and connection lifetime through `conf/database.yml`; do not hardcode environment-specific values.
- Close partially initialized resources on every startup failure path.

## Transactions

- Keep transaction boundaries explicit. Use `Store.Tx` and execute transactional SQL through `Store.SQL(txCtx)`, where `txCtx` is the context received by the transaction callback.
- Pass that callback context to every operation and downstream helper participating in the transaction. Using the outer context or calling `Store.DB` directly can execute SQL outside the transaction.
- Return operation errors from the callback so `Store.Tx` can roll back; a nil callback result allows it to commit. Propagate the error returned by `Store.Tx`, including commit failures.
- `Store.Tx` does not reuse an existing transaction or provide savepoints for nested calls. Open the transaction at the coordinating operation; helpers join it through the supplied context and `Store.SQL(ctx)` instead of calling `Store.Tx` again.

Correct: both executor selection and the operation use the callback context.

```go
return store.Tx(ctx, func(txCtx context.Context) error {
    _, err := store.SQL(txCtx).ExecContext(txCtx, query, args...)
    return err
})
```

Incorrect: the outer context selects a non-transactional executor when it has no transaction; direct `store.DB` calls also bypass the callback's transaction.

```go
return store.Tx(ctx, func(txCtx context.Context) error {
    _, err := store.SQL(ctx).ExecContext(ctx, query, args...)
    // store.DB.ExecContext(txCtx, query, args...) also bypasses the transaction.
    return err
})
```

## Schema

- Express every schema change as a SQL migration embedded in the service.
- Give each table an independent primary key. Use `BIGSERIAL PRIMARY KEY` for PostgreSQL unless the schema has a stronger established convention.
- Keep business identity separate from the primary key and enforce natural or composite keys with named unique constraints or indexes.
- Name business tables with `t_` followed by a singular noun in snake case, such as `t_user` or `t_user_group`.
- Every business table must place `created_at`, then `updated_at`, immediately after `id`; both use `TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP`.
- On updates, preserve `created_at` and explicitly set `updated_at` in SQL; the default applies only to inserts. Use UTC for application-supplied timestamps.
- Name indexes and constraints with descriptive prefixes such as `idx_`, `uk_`, `ck_`, and `fk_`.
- Avoid reserved or ambiguous identifiers when practical. If a reserved name is intentional, quote it consistently for the selected database dialect.
- Declare nullability, defaults, uniqueness, referential integrity, and domain checks explicitly in SQL.
- Add comments for non-obvious identifiers, state fields, counters, timestamps, and JSON columns.
- Prefer database constraints over application-only validation for data integrity.

## Migrations

- Use `YYYYMMDDHH-NN-description.sql`: a ten-digit year/month/day/hour prefix, a two-digit sequence starting at `01` each hour (including the first file), and lowercase hyphen-separated description words.
- Replace the template placeholder `YYYYMMDDHH` with an actual timestamp in service filenames. For example, `2026092314-01-create-user.sql` precedes `2026092314-02-seed-user.sql`; sequence numbers follow dependencies, not description text.
- Keep filenames unique and sort new migrations after their dependencies. Never edit, rename, or renumber committed migrations; add a new migration for corrections or extensions. Use a later hour prefix after sequence `99` or when depending on an older unnumbered migration.
- Include both `-- +migrate Up` and `-- +migrate Down` sections. Keep `Down` symmetrical with `Up`, removing dependent objects before the object introduced by the migration.
- When the embedded migrations directory contains no `.sql` files, migration execution must be skipped without touching the database handle.
- An uncommitted migration may be revised with its related code. Keep one table's complete initial definition together instead of immediately adding follow-up alterations.
- Keep migrations simple and readable. Seed only data required for local development or first boot.

## Security and Observability

- Never log database credentials, raw SQL arguments, or sensitive query results.
- Never log a raw DSN. Use `redact.DSN` whenever a DSN appears in a log message.
- Keep SQL values parameterized. Never build values into SQL strings; dynamic identifiers must be validated and quoted.
- Database log messages follow the [root logging rules](../../../AGENTS.md#logging) and include trace correlation when a request context is available.

## Testing

- Put tests requiring a real database in `internal/tests` and use an isolated test database.
