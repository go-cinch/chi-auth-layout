-- +migrate Up
ALTER TABLE t_audit_event
    ADD COLUMN IF NOT EXISTS header JSONB NOT NULL DEFAULT 'null'::jsonb;

-- +migrate StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 't_audit_event'::regclass AND conname = 'ck_audit_event_header'
    ) THEN
        ALTER TABLE t_audit_event
            ADD CONSTRAINT ck_audit_event_header CHECK (jsonb_typeof(header) IN ('object', 'null'));
    END IF;
END $$;
-- +migrate StatementEnd

COMMENT ON COLUMN t_audit_event.header IS 'Request headers for login and operation audit; sensitive values fully redacted; historical rows have no snapshot';

-- +migrate Down
ALTER TABLE t_audit_event
    DROP CONSTRAINT IF EXISTS ck_audit_event_header,
    DROP COLUMN IF EXISTS header;
