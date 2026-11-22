-- +migrate Up
-- Catalog consumers resolve labels through I18N_CATALOG and transport rules through config.
-- +migrate StatementBegin
CREATE FUNCTION action_resource_bindings_valid(bindings JSONB, required_scope TEXT)
RETURNS BOOLEAN
LANGUAGE SQL IMMUTABLE
AS $$
    SELECT COALESCE(JSONB_TYPEOF(bindings) = 'array', FALSE)
        AND NOT EXISTS (
            SELECT 1
            FROM JSONB_ARRAY_ELEMENTS(
                CASE WHEN JSONB_TYPEOF(bindings) = 'array' THEN bindings ELSE '[]'::JSONB END
            ) AS item(binding)
            WHERE (
                JSONB_TYPEOF(binding) = 'object'
                AND JSONB_TYPEOF(binding -> 'resource_key') = 'string'
                AND binding ->> 'resource_key' ~ '^[a-z][a-z0-9_-]*(:[a-z][a-z0-9_-]*){2,}$'
                AND SPLIT_PART(binding ->> 'resource_key', ':', 1) = required_scope
                AND JSONB_TYPEOF(binding -> 'operation') = 'string'
                AND binding ->> 'operation' IN ('read', 'create', 'update', 'delete', '*')
                AND (
                    NOT (binding ? 'feature_keys')
                    OR JSONB_TYPEOF(binding -> 'feature_keys') IN ('array', 'null')
                )
            ) IS NOT TRUE
        )
        AND (
            SELECT COUNT(*) = COUNT(DISTINCT (binding ->> 'resource_key', binding ->> 'operation'))
            FROM JSONB_ARRAY_ELEMENTS(
                CASE WHEN JSONB_TYPEOF(bindings) = 'array' THEN bindings ELSE '[]'::JSONB END
            ) AS item(binding)
        );
$$;
-- +migrate StatementEnd

CREATE TABLE t_action (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    action_group VARCHAR(128) NOT NULL,
    code CHAR(8) NOT NULL,
    word VARCHAR(128) NOT NULL,
    scope TEXT GENERATED ALWAYS AS (SPLIT_PART(word, ':', 1)) STORED NOT NULL,
    config JSONB NOT NULL,
    CONSTRAINT uk_action_code UNIQUE (code),
    CONSTRAINT uk_action_word UNIQUE (word),
    CONSTRAINT ck_action_group CHECK (
        action_group ~ '^[a-z][a-z0-9_-]*(:[a-z][a-z0-9_-]*){2,}$'
        AND SPLIT_PART(action_group, ':', 1) = SPLIT_PART(word, ':', 1)
    ),
    CONSTRAINT ck_action_code CHECK (CHAR_LENGTH(TRIM(code)) = 8),
    CONSTRAINT ck_action_word_format CHECK (
        word ~ '^[a-z][a-z0-9_-]*(:[a-z][a-z0-9_-]*){3,}$'
    ),
    CONSTRAINT ck_action_config_shape CHECK (
        JSONB_TYPEOF(config) = 'object'
        AND config ?& ARRAY['ui', 'endpoints']
        AND JSONB_TYPEOF(config -> 'ui') = 'array'
        AND JSONB_TYPEOF(config -> 'endpoints') = 'array'
    ),
    CONSTRAINT ck_action_grpc_method CHECK (
        NOT JSONB_PATH_EXISTS(
            config,
            '$.endpoints[*] ? (@.protocol == "grpc" && (!exists(@.method) || @.method.type() != "string" || @.method != "GRPC"))'
        )
    ),
    CONSTRAINT ck_action_builtin CHECK (
        NOT (config ? 'builtin')
        OR (
            JSONB_TYPEOF(config -> 'builtin') = 'string'
            AND config ->> 'builtin' IN ('all', 'default')
            AND config ->> 'builtin' = SPLIT_PART(word, ':', 2)
        )
    ),
    CONSTRAINT ck_action_all_config CHECK (
        config ->> 'builtin' IS DISTINCT FROM 'all'
        OR (config -> 'ui' = '[]'::jsonb AND config -> 'endpoints' = '[]'::jsonb)
    ),
    CONSTRAINT ck_action_resource_binding CHECK ((
        NOT (config ? 'resource_key') AND NOT (config ? 'operation')
        AND CASE
            WHEN config ->> 'builtin' IN ('all', 'default') THEN NOT (config ? 'bindings')
            ELSE action_resource_bindings_valid(config -> 'bindings', scope)
        END
    ) IS TRUE)
);

COMMENT ON TABLE t_action IS 'Scoped permission catalog with UI and transport mappings; labels reside in I18N_CATALOG';

COMMENT ON COLUMN t_action.created_at IS 'Row creation time in Unix milliseconds';

COMMENT ON COLUMN t_action.updated_at IS 'Last permission update time in Unix milliseconds';

COMMENT ON COLUMN t_action.action_group IS 'Stable scope:module:resource group identifier; label lookup uses frontend.groups[action_group]';

COMMENT ON COLUMN t_action.code IS 'Stable eight-character assignment code used by users, roles and user groups';

COMMENT ON COLUMN t_action.word IS 'Scope:operation:module:resource permission identifier; code is the stable assignment identity; label lookup uses frontend.actions[word]';

COMMENT ON COLUMN t_action.scope IS 'Stored scope derived from word; label lookup uses frontend.scopes[scope]';

COMMENT ON COLUMN t_action.config IS 'Explicit resource/operation bindings select logical feature keys; ui and endpoints are derived projections. builtin=all grants the scope; builtin=default selects explicit resources and features';

CREATE INDEX idx_action_scope_group ON t_action (scope, action_group);

-- Built-in kinds are reserved catalog definitions, with at most one of each per scope.
CREATE UNIQUE INDEX uk_action_scope_builtin ON t_action (scope, (config ->> 'builtin'))
    WHERE config ? 'builtin';

CREATE TABLE t_role (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    name VARCHAR(50) NOT NULL,
    word VARCHAR(50) NOT NULL,
    action TEXT NOT NULL DEFAULT '',
    CONSTRAINT uk_role_word UNIQUE (word),
    CONSTRAINT ck_role_name CHECK (CHAR_LENGTH(TRIM(name)) > 0),
    CONSTRAINT ck_role_word CHECK (CHAR_LENGTH(TRIM(word)) > 0)
);

COMMENT ON TABLE t_role IS 'user roles';

COMMENT ON COLUMN t_role.created_at IS 'Row creation time in Unix milliseconds';

COMMENT ON COLUMN t_role.updated_at IS 'Last role update time in Unix milliseconds';

COMMENT ON COLUMN t_role.word IS 'unique role keyword used by clients';

COMMENT ON COLUMN t_role.action IS 'comma-separated action codes';

CREATE TABLE t_user_group (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    name VARCHAR(50) NOT NULL,
    word VARCHAR(50) NOT NULL,
    action TEXT NOT NULL DEFAULT '',
    CONSTRAINT uk_user_group_word UNIQUE (word),
    CONSTRAINT ck_user_group_name CHECK (CHAR_LENGTH(TRIM(name)) > 0),
    CONSTRAINT ck_user_group_word CHECK (CHAR_LENGTH(TRIM(word)) > 0)
);

COMMENT ON TABLE t_user_group IS 'user permission groups';

COMMENT ON COLUMN t_user_group.created_at IS 'Row creation time in Unix milliseconds';

COMMENT ON COLUMN t_user_group.updated_at IS 'Last user group update time in Unix milliseconds';

COMMENT ON COLUMN t_user_group.word IS 'unique group keyword used by clients';

COMMENT ON COLUMN t_user_group.action IS 'comma-separated action codes';

CREATE TABLE t_user (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    role_id BIGINT NULL,
    action TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL,
    email TEXT,
    email_verified_at BIGINT,
    totp_ever_bound BOOLEAN DEFAULT FALSE,
    totp_recovery_pending BOOLEAN NOT NULL DEFAULT FALSE,
    code CHAR(8) NOT NULL,
    password TEXT NOT NULL,
    last_logged_in_at BIGINT NULL,
    status SMALLINT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    wrong BIGINT NOT NULL DEFAULT 0,
    credential_version BIGINT NOT NULL DEFAULT 1,
    login_count BIGINT NOT NULL DEFAULT 0,
    admin_password_reset_pending BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT uk_user_username UNIQUE (username),
    CONSTRAINT uk_user_email UNIQUE (email),
    CONSTRAINT ck_user_email CHECK (email IS NULL OR (email = LOWER(TRIM(email)) AND email <> '')),
    CONSTRAINT uk_user_code UNIQUE (code),
    CONSTRAINT fk_user_role FOREIGN KEY (role_id) REFERENCES t_role (id) ON DELETE SET NULL,
    CONSTRAINT ck_user_username CHECK (CHAR_LENGTH(TRIM(username)) > 0),
    CONSTRAINT ck_user_code CHECK (CHAR_LENGTH(TRIM(code)) = 8),
    CONSTRAINT ck_user_password CHECK (CHAR_LENGTH(password) > 0),
    CONSTRAINT ck_user_status CHECK (status IN (0, 1, 2)),
    CONSTRAINT ck_user_metadata CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT ck_user_lock_metadata CHECK (
        (status = 2 AND metadata ? 'lock_expired_at' AND jsonb_typeof(metadata -> 'lock_expired_at') = 'number' AND (metadata ->> 'lock_expired_at') ~ '^[0-9]+$')
        OR (status <> 2 AND NOT (metadata ? 'lock_expired_at'))
    ),
    CONSTRAINT ck_user_credential_version CHECK (credential_version > 0),
    CONSTRAINT ck_user_login_count CHECK (login_count >= 0),
    CONSTRAINT ck_user_admin_password_reset_pending CHECK (NOT admin_password_reset_pending OR login_count = 0),
    CONSTRAINT ck_user_wrong CHECK (wrong >= 0)
);

COMMENT ON TABLE t_user IS 'authentication users';

COMMENT ON COLUMN t_user.created_at IS 'Account creation time in Unix milliseconds';

COMMENT ON COLUMN t_user.updated_at IS 'Last account update time in Unix milliseconds';

COMMENT ON COLUMN t_user.role_id IS 'optional role id';

COMMENT ON COLUMN t_user.action IS 'comma-separated user-specific action codes';

COMMENT ON COLUMN t_user.username IS 'unique login name';

COMMENT ON COLUMN t_user.email IS 'Verified canonical email login alias; NULL until binding is confirmed';

COMMENT ON COLUMN t_user.email_verified_at IS 'Unix milliseconds when email possession was verified';

COMMENT ON COLUMN t_user.totp_ever_bound IS 'Whether an authenticator was ever bound; NULL means unknown historical enrollment and forbids password-only unlinking';

COMMENT ON COLUMN t_user.totp_recovery_pending IS 'Whether authenticator recovery is awaiting replacement enrollment';

COMMENT ON COLUMN t_user.code IS 'stable eight-character user code';

COMMENT ON COLUMN t_user.password IS 'adaptive password hash';

COMMENT ON COLUMN t_user.last_logged_in_at IS 'Last successful login time in Unix milliseconds; null before the first successful login';

COMMENT ON COLUMN t_user.status IS 'account state: 0 pending approval, 1 active, 2 locked';

COMMENT ON COLUMN t_user.metadata IS 'extensible user attributes; lock_expired_at is Unix milliseconds and zero means a permanent lock; password_change_failures disables self-service password changes at the configured threshold';

COMMENT ON COLUMN t_user.wrong IS 'consecutive wrong-password count';

COMMENT ON COLUMN t_user.credential_version IS 'Incremented atomically on password changes to invalidate all previously issued sessions';

COMMENT ON COLUMN t_user.login_count IS 'Completed logins since account creation or the last administrator password reset; zero selects the configured onboarding password policy';

COMMENT ON COLUMN t_user.admin_password_reset_pending IS 'Whether the outstanding password step follows an administrator reset and uses the administrator-reset policy';

CREATE INDEX idx_user_role_id ON t_user (role_id);

CREATE TABLE t_user_totp (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    user_id BIGINT NOT NULL,
    secret_ciphertext TEXT NOT NULL,
    last_used_step BIGINT NOT NULL,
    period_seconds BIGINT NOT NULL,
    CONSTRAINT ck_user_totp_period CHECK (period_seconds BETWEEN 1 AND 9223372036),
    CONSTRAINT uk_user_totp_user UNIQUE (user_id),
    CONSTRAINT fk_user_totp_user FOREIGN KEY (user_id) REFERENCES t_user (id) ON DELETE CASCADE,
    CONSTRAINT ck_user_totp_secret CHECK (CHAR_LENGTH(secret_ciphertext) > 0),
    CONSTRAINT ck_user_totp_step CHECK (last_used_step >= 0)
);

COMMENT ON TABLE t_user_totp IS 'One confirmed authenticator per user; pending enrollment is stored separately in Redis';

COMMENT ON COLUMN t_user_totp.created_at IS 'Authenticator binding time in Unix milliseconds; the row is created only after a valid TOTP';

COMMENT ON COLUMN t_user_totp.updated_at IS 'Last accepted authenticator verification time in Unix milliseconds';

COMMENT ON COLUMN t_user_totp.secret_ciphertext IS 'AES-256-GCM encrypted TOTP secret, authenticated against its user ID; never return in user APIs';

COMMENT ON COLUMN t_user_totp.last_used_step IS 'Last accepted counter at the configured TOTP period; initialized by enrollment and updated under row lock to reject replay';

COMMENT ON COLUMN t_user_totp.period_seconds IS 'TOTP time step in whole seconds fixed at enrollment; independent of deployment defaults';

CREATE TABLE t_user_recovery_code (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    user_id BIGINT NOT NULL,
    code_hash TEXT NOT NULL,
    CONSTRAINT fk_user_recovery_code_user FOREIGN KEY (user_id) REFERENCES t_user (id) ON DELETE CASCADE,
    CONSTRAINT uk_user_recovery_code UNIQUE (user_id, code_hash),
    CONSTRAINT ck_user_recovery_code_hash CHECK (code_hash ~ '^[a-f0-9]{64}$')
);

COMMENT ON TABLE t_user_recovery_code IS 'Current authenticator recovery codes; successful recovery deletes the entire set and the bound factor in one transaction';

COMMENT ON COLUMN t_user_recovery_code.created_at IS 'Recovery code issuance time in Unix milliseconds';

COMMENT ON COLUMN t_user_recovery_code.updated_at IS 'Issuance time in Unix milliseconds; recovery codes are replaced or deleted, never updated';

COMMENT ON COLUMN t_user_recovery_code.code_hash IS 'SHA-256 digest of a cryptographically random 128-bit recovery code; plaintext shown only at enrollment';

CREATE TABLE t_user_user_group_relation (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    user_id BIGINT NOT NULL,
    user_group_id BIGINT NOT NULL,
    CONSTRAINT uk_user_user_group_relation UNIQUE (user_id, user_group_id),
    CONSTRAINT fk_user_user_group_relation_user FOREIGN KEY (user_id) REFERENCES t_user (id) ON DELETE CASCADE,
    CONSTRAINT fk_user_user_group_relation_group FOREIGN KEY (user_group_id) REFERENCES t_user_group (id) ON DELETE CASCADE
);

COMMENT ON TABLE t_user_user_group_relation IS 'many-to-many relation between users and user groups';

COMMENT ON COLUMN t_user_user_group_relation.created_at IS 'Membership creation time in Unix milliseconds';

COMMENT ON COLUMN t_user_user_group_relation.updated_at IS 'Last membership update time in Unix milliseconds';

COMMENT ON COLUMN t_user_user_group_relation.user_id IS 'related user id';

COMMENT ON COLUMN t_user_user_group_relation.user_group_id IS 'related user group id';

CREATE INDEX idx_user_group_member
    ON t_user_user_group_relation (user_group_id, user_id);

CREATE TABLE t_dictionary (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    dictionary_key VARCHAR(100) NOT NULL,
    name VARCHAR(100) NOT NULL,
    value JSONB NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    CONSTRAINT uk_dictionary_key UNIQUE (dictionary_key),
    CONSTRAINT ck_dictionary_key CHECK (dictionary_key ~ '^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$'),
    CONSTRAINT ck_dictionary_name CHECK (CHAR_LENGTH(TRIM(name)) > 0)
);

COMMENT ON TABLE t_dictionary IS 'dynamic system constants and data dictionaries';

COMMENT ON COLUMN t_dictionary.created_at IS 'Row creation time in Unix milliseconds';

COMMENT ON COLUMN t_dictionary.updated_at IS 'Last dictionary update time in Unix milliseconds';

COMMENT ON COLUMN t_dictionary.dictionary_key IS 'stable uppercase key with underscore-separated segments';

COMMENT ON COLUMN t_dictionary.value IS 'arbitrary JSON dictionary value';

COMMENT ON COLUMN t_dictionary.description IS 'administrator-facing purpose and usage notes';

COMMENT ON COLUMN t_dictionary.enabled IS 'whether runtime consumers may read this dictionary';

CREATE TABLE t_msg (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    title VARCHAR(200) NOT NULL,
    content TEXT NOT NULL,
    type VARCHAR(20) NOT NULL,
    scope VARCHAR(20) NOT NULL,
    sender_id BIGINT NULL,
    published_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    expired_at BIGINT NULL,
    CONSTRAINT fk_msg_sender FOREIGN KEY (sender_id) REFERENCES t_user (id) ON DELETE SET NULL,
    CONSTRAINT ck_msg_title CHECK (CHAR_LENGTH(TRIM(title)) > 0),
    CONSTRAINT ck_msg_content CHECK (CHAR_LENGTH(TRIM(content)) BETWEEN 1 AND 20000),
    CONSTRAINT ck_msg_type CHECK (type IN ('system', 'notice')),
    CONSTRAINT ck_msg_scope CHECK (scope IN ('all', 'targeted')),
    CONSTRAINT ck_msg_expired_at CHECK (expired_at IS NULL OR expired_at > published_at)
);

COMMENT ON COLUMN t_msg.created_at IS 'Message creation time in Unix milliseconds';

COMMENT ON COLUMN t_msg.updated_at IS 'Last message update time in Unix milliseconds';

COMMENT ON COLUMN t_msg.published_at IS 'Publication time in Unix milliseconds; a future value schedules visibility';

COMMENT ON COLUMN t_msg.expired_at IS 'Exclusive expiry time in Unix milliseconds; null means the message does not expire';

COMMENT ON COLUMN t_msg.scope IS 'all: shared broadcast visible to users registered by publication; targeted: recipients inserted at send time';

COMMENT ON COLUMN t_msg.sender_id IS 'verified sender; null after sender account deletion';

CREATE INDEX idx_msg_published_at ON t_msg (published_at DESC, id DESC);

CREATE INDEX idx_msg_broadcast ON t_msg (published_at DESC, id DESC) WHERE scope = 'all';

CREATE TABLE t_msg_send_receipt (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    sender_id BIGINT NOT NULL,
    request_key VARCHAR(128) NOT NULL,
    payload_hash BYTEA NOT NULL,
    msg_id BIGINT NULL,
    CONSTRAINT fk_msg_send_receipt_sender FOREIGN KEY (sender_id) REFERENCES t_user (id) ON DELETE CASCADE,
    CONSTRAINT fk_msg_send_receipt_msg FOREIGN KEY (msg_id) REFERENCES t_msg (id) ON DELETE SET NULL,
    CONSTRAINT uk_msg_send_receipt_sender_key UNIQUE (sender_id, request_key),
    CONSTRAINT ck_msg_send_receipt_request_key CHECK (request_key COLLATE "C" ~ '^[A-Za-z0-9_.:-]+$'),
    CONSTRAINT ck_msg_send_receipt_payload_hash CHECK (OCTET_LENGTH(payload_hash) = 32)
);

COMMENT ON TABLE t_msg_send_receipt IS 'Records the result of an idempotent message send, including after withdrawal';

COMMENT ON COLUMN t_msg_send_receipt.created_at IS 'Time the sender claimed the request key in Unix milliseconds';

COMMENT ON COLUMN t_msg_send_receipt.updated_at IS 'Last receipt change in Unix milliseconds, including withdrawal';

COMMENT ON COLUMN t_msg_send_receipt.request_key IS 'Client-supplied X-Idempotent key scoped to the sender';

COMMENT ON COLUMN t_msg_send_receipt.payload_hash IS 'SHA-256 digest of the normalized send request used to detect conflicting retries';

COMMENT ON COLUMN t_msg_send_receipt.msg_id IS 'Set on publication and cleared on withdrawal so a retry cannot republish the message';

CREATE INDEX idx_msg_send_receipt_msg ON t_msg_send_receipt (msg_id) WHERE msg_id IS NOT NULL;

-- ON DELETE SET NULL also changes the receipt when the message is withdrawn.
-- +migrate StatementBegin
CREATE FUNCTION msg_send_receipt_withdrawn() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint;
    RETURN NEW;
END;
$$;
-- +migrate StatementEnd

CREATE TRIGGER msg_send_receipt_withdrawn BEFORE UPDATE OF msg_id ON t_msg_send_receipt
    FOR EACH ROW WHEN (OLD.msg_id IS NOT NULL AND NEW.msg_id IS NULL)
    EXECUTE FUNCTION msg_send_receipt_withdrawn();

CREATE TABLE t_msg_recipient (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    msg_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    read_at BIGINT NULL,
    deleted_at BIGINT NULL,
    CONSTRAINT fk_msg_recipient_message FOREIGN KEY (msg_id) REFERENCES t_msg (id) ON DELETE CASCADE,
    CONSTRAINT fk_msg_recipient_user FOREIGN KEY (user_id) REFERENCES t_user (id) ON DELETE CASCADE,
    CONSTRAINT uk_msg_recipient UNIQUE (msg_id, user_id)
);

COMMENT ON TABLE t_msg_recipient IS 'targeted delivery records and sparse per-user broadcast states; a missing broadcast state means unread';

COMMENT ON COLUMN t_msg_recipient.created_at IS 'Delivery or personal state creation time in Unix milliseconds';

COMMENT ON COLUMN t_msg_recipient.updated_at IS 'Last personal message state update time in Unix milliseconds';

COMMENT ON COLUMN t_msg_recipient.read_at IS 'Read time in Unix milliseconds; null means unread';

COMMENT ON COLUMN t_msg_recipient.deleted_at IS 'Personal deletion time in Unix milliseconds; null means not deleted; the tombstone prevents a deleted broadcast from reappearing';

CREATE INDEX idx_msg_recipient_user ON t_msg_recipient (user_id, msg_id);

CREATE INDEX idx_msg_recipient_unread ON t_msg_recipient (user_id, msg_id) WHERE read_at IS NULL AND deleted_at IS NULL;

CREATE TABLE t_user_msg_status (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    user_id BIGINT NOT NULL,
    unread_count BIGINT NOT NULL DEFAULT 0,
    latest_unread_at BIGINT NULL,
    next_refresh_at BIGINT NULL,
    CONSTRAINT uk_user_msg_status_user UNIQUE (user_id),
    CONSTRAINT fk_user_msg_status_user FOREIGN KEY (user_id) REFERENCES t_user (id) ON DELETE CASCADE,
    CONSTRAINT ck_user_msg_status_count CHECK (unread_count >= 0),
    CONSTRAINT ck_user_msg_status_latest CHECK ((unread_count = 0) = (latest_unread_at IS NULL))
);

COMMENT ON TABLE t_user_msg_status IS 'Persisted inbox statistics; message writes update these in the same transaction';

COMMENT ON COLUMN t_user_msg_status.id IS 'Independent row identifier';

COMMENT ON COLUMN t_user_msg_status.created_at IS 'Row creation time in Unix milliseconds';

COMMENT ON COLUMN t_user_msg_status.updated_at IS 'Last statistics update time in Unix milliseconds';

COMMENT ON COLUMN t_user_msg_status.user_id IS 'Unique owner; removed when the user is deleted';

COMMENT ON COLUMN t_user_msg_status.unread_count IS 'Count of visible, unread, undeleted, unexpired messages';

COMMENT ON COLUMN t_user_msg_status.latest_unread_at IS 'Latest publication Unix milliseconds among unread messages; null exactly when count is zero';

COMMENT ON COLUMN t_user_msg_status.next_refresh_at IS 'Next unread publication or expiry in Unix milliseconds; null means no scheduled refresh';

CREATE INDEX idx_user_msg_status_refresh ON t_user_msg_status (next_refresh_at, user_id) WHERE next_refresh_at IS NOT NULL;

CREATE TABLE t_audit_event (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    occurred_at BIGINT NOT NULL,
    kind VARCHAR(16) NOT NULL,
    user_id BIGINT NULL,
    username VARCHAR(256) NOT NULL DEFAULT '',
    module VARCHAR(64) NOT NULL,
    method VARCHAR(16) NOT NULL,
    path VARCHAR(1024) NOT NULL,
    ip VARCHAR(128) NOT NULL DEFAULT '',
    user_agent VARCHAR(512) NOT NULL DEFAULT '',
    trace_id VARCHAR(128) NOT NULL DEFAULT '',
    error_code VARCHAR(128) NOT NULL DEFAULT '',
    status INTEGER NOT NULL,
    duration_ms BIGINT NOT NULL,
    operation VARCHAR(32) NOT NULL DEFAULT '',
    resource_type VARCHAR(64) NOT NULL DEFAULT '',
    resource_id VARCHAR(1024) NOT NULL DEFAULT '',
    request_params JSONB NOT NULL DEFAULT 'null'::jsonb,
    response_params JSONB NOT NULL DEFAULT 'null'::jsonb,
    header JSONB NOT NULL DEFAULT 'null'::jsonb,
    step_up_fingerprint TEXT NOT NULL DEFAULT '',
    step_up_verified_at BIGINT NULL,
    delivery_id TEXT NULL,
    CONSTRAINT ck_audit_event_delivery_id CHECK (delivery_id <> ''),
    CONSTRAINT ck_audit_event_kind CHECK (kind IN ('login', 'operation')),
    CONSTRAINT ck_audit_event_user CHECK (user_id IS NULL OR user_id > 0),
    CONSTRAINT ck_audit_event_status CHECK (status BETWEEN 100 AND 599),
    CONSTRAINT ck_audit_event_duration CHECK (duration_ms >= 0),
    CONSTRAINT ck_audit_event_header CHECK (jsonb_typeof(header) IN ('object', 'null'))
);

COMMENT ON TABLE t_audit_event IS 'Append-only login attempts and business HTTP mutations; only sanitized parameters, never raw secrets or file contents';

COMMENT ON COLUMN t_audit_event.id IS 'Independent row identifier';

COMMENT ON COLUMN t_audit_event.created_at IS 'Persistence time in Unix milliseconds';

COMMENT ON COLUMN t_audit_event.updated_at IS 'Persistence time in Unix milliseconds; audit records are not edited';

COMMENT ON COLUMN t_audit_event.occurred_at IS 'Request start time in Unix milliseconds';

COMMENT ON COLUMN t_audit_event.kind IS 'login: login attempt; operation: business HTTP mutation';

COMMENT ON COLUMN t_audit_event.user_id IS 'Verified actor ID, retained after user deletion; null for anonymous attempts';

COMMENT ON COLUMN t_audit_event.username IS 'Account snapshot; attempted account on failed login, not proof of verified identity';

COMMENT ON COLUMN t_audit_event.module IS 'Owning business capability';

COMMENT ON COLUMN t_audit_event.method IS 'HTTP request method';

COMMENT ON COLUMN t_audit_event.path IS 'Request path without query string';

COMMENT ON COLUMN t_audit_event.ip IS 'Client IP from X-Verified-Client-IP, otherwise transport peer IP; source trust is enforced by deployment';

COMMENT ON COLUMN t_audit_event.user_agent IS 'Bounded client user-agent snapshot';

COMMENT ON COLUMN t_audit_event.trace_id IS 'Request trace correlation identifier, when tracing is enabled';

COMMENT ON COLUMN t_audit_event.error_code IS 'Stable public error code; empty for responses without a code';

COMMENT ON COLUMN t_audit_event.status IS 'HTTP response status; 200 through 399 is classified as success';

COMMENT ON COLUMN t_audit_event.duration_ms IS 'HTTP handler elapsed milliseconds, excluding audit persistence';

COMMENT ON COLUMN t_audit_event.operation IS 'Business operation inferred from the registered audit target';

COMMENT ON COLUMN t_audit_event.resource_type IS 'Type of the business object being acted on';

COMMENT ON COLUMN t_audit_event.resource_id IS 'Target object identifier or comma-separated identifiers when available';

COMMENT ON COLUMN t_audit_event.request_params IS 'Sanitized query and body; secrets hidden, files represented by file, unsafe or oversized bodies omitted';

COMMENT ON COLUMN t_audit_event.response_params IS 'Sanitized response body; secrets hidden, files represented by file, unsafe or oversized bodies omitted';

COMMENT ON COLUMN t_audit_event.header IS 'Request headers for login and operation audit; sensitive values fully redacted; historical rows have no snapshot';

COMMENT ON COLUMN t_audit_event.step_up_fingerprint IS 'Opaque identifier of the sensitive-operation verification reused by this session; never derived from a secret or OTP';

COMMENT ON COLUMN t_audit_event.step_up_verified_at IS 'Unix milliseconds of the sensitive-operation verification, not login verification';

COMMENT ON COLUMN t_audit_event.delivery_id IS 'Producer-generated Redis delivery identifier shared by events from one delivery, NULL for synchronous writes';

CREATE UNIQUE INDEX uk_audit_event_delivery_kind ON t_audit_event (delivery_id, kind) WHERE delivery_id IS NOT NULL;

CREATE INDEX idx_audit_event_kind_time ON t_audit_event (kind, occurred_at DESC, id DESC);

CREATE INDEX idx_audit_event_username_time ON t_audit_event (username, occurred_at DESC);

CREATE TABLE t_auth_email_code (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    verification_id TEXT NOT NULL,
    browser_uuid UUID,
    client_ip TEXT NOT NULL,
    purpose TEXT NOT NULL,
    identifier TEXT NOT NULL,
    user_id BIGINT,
    recipient TEXT NOT NULL,
    credential_version BIGINT NOT NULL,
    code_hash TEXT NOT NULL,
    expired_at BIGINT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    delivered BOOLEAN NOT NULL DEFAULT FALSE,
    consumed BOOLEAN NOT NULL DEFAULT FALSE,
    delivery_attempts SMALLINT,
    delivery_after BIGINT NOT NULL DEFAULT 0,
    unlink_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
    notice_detail TEXT NOT NULL DEFAULT '',
    delivery_account TEXT NOT NULL DEFAULT '',
    delivery_language TEXT NOT NULL DEFAULT '',
    delivery_mac TEXT NOT NULL DEFAULT '',
    CONSTRAINT uk_auth_email_code_verification_id UNIQUE (verification_id),
    CONSTRAINT fk_auth_email_code_user FOREIGN KEY (user_id) REFERENCES t_user (id) ON DELETE CASCADE,
    CONSTRAINT ck_auth_email_code_purpose CHECK (purpose IN ('login', 'bind', 'sensitive', 'unlink', 'unlink_notice', 'totp_bound', 'totp_rebound', 'sensitive_notice', 'totp_reset', 'email_linked')),
    CONSTRAINT ck_auth_email_code_attempts CHECK (attempts >= 0),
    CONSTRAINT ck_auth_email_code_delivery_attempts CHECK (delivery_attempts >= 0)
);

CREATE INDEX idx_auth_email_code_browser ON t_auth_email_code (browser_uuid, purpose, created_at DESC);

CREATE INDEX idx_auth_email_code_expiry ON t_auth_email_code (expired_at);

CREATE INDEX idx_auth_email_code_user ON t_auth_email_code (user_id);

CREATE TABLE t_auth_email_cooldown (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    cooldown_key TEXT NOT NULL,
    retry_at BIGINT NOT NULL,
    CONSTRAINT uk_auth_email_cooldown_key UNIQUE (cooldown_key)
);

CREATE INDEX idx_auth_email_cooldown_retry ON t_auth_email_cooldown (retry_at, id);

COMMENT ON TABLE t_auth_email_cooldown IS 'Shared atomic send budgets for browser UUID, source IP and destination/account';

COMMENT ON COLUMN t_auth_email_code.verification_id IS 'Opaque verification ticket shared with clients and delivery jobs; independent of the row primary key';

COMMENT ON COLUMN t_auth_email_cooldown.cooldown_key IS 'Browser, source IP or recipient send-budget key';

COMMENT ON COLUMN t_auth_email_code.delivery_attempts IS 'NULL for tickets without queued delivery; otherwise the number of reserved SMTP attempts';

COMMENT ON COLUMN t_auth_email_code.delivery_after IS 'Next eligible delivery or active worker lease deadline, Unix milliseconds';

COMMENT ON COLUMN t_auth_email_code.notice_detail IS 'Notification rendering details; sensitive-operation summaries contain versioned JSON';

COMMENT ON COLUMN t_auth_email_code.delivery_mac IS 'HMAC of immutable delivery inputs; workers reject unsigned or modified payloads';

CREATE INDEX idx_auth_email_unlink_owner ON t_auth_email_code (user_id, created_at DESC)
    WHERE purpose = 'unlink' AND NOT consumed;

-- +migrate StatementBegin
CREATE FUNCTION auth_email_alias_check() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(713642869);
    IF EXISTS (
        SELECT 1 FROM t_user u WHERE u.id <> NEW.id
            AND (u.email = LOWER(TRIM(NEW.username)) OR (NEW.email IS NOT NULL AND LOWER(TRIM(u.username)) = NEW.email))
    ) THEN
        RAISE unique_violation USING CONSTRAINT = 'uk_user_login_alias', MESSAGE = 'login alias is already in use';
    END IF;
    RETURN NEW;
END;
$$;
-- +migrate StatementEnd

CREATE TRIGGER auth_email_alias_check BEFORE INSERT OR UPDATE OF username, email ON t_user
    FOR EACH ROW EXECUTE FUNCTION auth_email_alias_check();

CREATE TABLE t_sensitive_operation (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    user_id BIGINT NOT NULL,
    scan_id BIGINT,
    operation_kind TEXT NOT NULL DEFAULT 'other',
    permission_related BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT ck_sensitive_operation_kind CHECK (operation_kind IN ('create', 'delete', 'update', 'other'))
);

CREATE INDEX idx_sensitive_operation_actor ON t_sensitive_operation (user_id, created_at, id);

CREATE INDEX idx_sensitive_operation_pending ON t_sensitive_operation (id) WHERE scan_id IS NULL;

CREATE INDEX idx_sensitive_operation_scan ON t_sensitive_operation (scan_id) WHERE scan_id IS NOT NULL;

COMMENT ON COLUMN t_sensitive_operation.scan_id IS 'Scan which evaluated this committed mutation; NULL until claimed';

CREATE TABLE t_sensitive_scan (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    slot BIGINT NOT NULL,
    CONSTRAINT uk_sensitive_scan_slot UNIQUE (slot)
);

CREATE INDEX idx_sensitive_scan_created ON t_sensitive_scan (created_at, id);

COMMENT ON COLUMN t_sensitive_operation.operation_kind IS 'Committed business operation category; action endpoints such as publication and credential binding count as update.';

COMMENT ON COLUMN t_sensitive_operation.permission_related IS 'Overlapping permission-related classification, including grant fields and permission catalog mutations.';

-- The official River migrations create river_job before the Auth migrations run.
-- +migrate StatementBegin
CREATE FUNCTION auth_enqueue_email(ticket_id TEXT, email_purpose TEXT) RETURNS VOID
LANGUAGE SQL AS $$
    INSERT INTO river_job (kind, queue, args, max_attempts)
    VALUES (
        'auth_email_deliver',
        CASE WHEN email_purpose IN ('unlink_notice', 'totp_bound', 'totp_rebound', 'sensitive_notice', 'email_linked')
            THEN 'auth_email_notice' ELSE 'auth_email_interactive' END,
        jsonb_build_object('ticket_id', ticket_id),
        25
    );
$$;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE FUNCTION auth_email_after_insert() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    PERFORM auth_enqueue_email(NEW.verification_id, NEW.purpose);
    RETURN NULL;
EXCEPTION WHEN OTHERS THEN
    RAISE EXCEPTION USING ERRCODE = 'AE001', MESSAGE = 'could not enqueue email delivery';
END;
$$;
-- +migrate StatementEnd

CREATE TRIGGER auth_email_after_insert
    AFTER INSERT ON t_auth_email_code
    FOR EACH ROW WHEN (NEW.delivery_attempts IS NOT NULL AND NOT NEW.delivered AND NOT NEW.consumed)
    EXECUTE FUNCTION auth_email_after_insert();

CREATE TABLE t_mail_template (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    template_id TEXT NOT NULL,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1,
    published_version BIGINT NOT NULL DEFAULT 0,
    has_draft BOOLEAN NOT NULL DEFAULT TRUE,
    content JSONB NOT NULL,
    CONSTRAINT uk_mail_template_template_id UNIQUE (template_id),
    CONSTRAINT ck_mail_template_category CHECK (category IN ('verification', 'confirmation', 'security', 'notification')),
    CONSTRAINT ck_mail_template_revision CHECK (revision > 0 AND published_version >= 0),
    CONSTRAINT ck_mail_template_content CHECK (jsonb_typeof(content) = 'object' AND content ? 'zh-CN' AND content ? 'en-US')
);

COMMENT ON COLUMN t_mail_template.template_id IS 'Editable unique business identifier; numeric primary key and publication history remain stable.';

COMMENT ON COLUMN t_mail_template.content IS 'Editable per-language draft subject, HTML, text and preview-only examples; delivery reads the published version.';

COMMENT ON COLUMN t_mail_template.revision IS 'Optimistic concurrency token incremented by every draft or publication change.';

CREATE TABLE t_mail_template_version (
    id BIGSERIAL PRIMARY KEY,
    created_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    updated_at BIGINT NOT NULL DEFAULT (FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint),
    template_pk BIGINT NOT NULL,
    version BIGINT NOT NULL,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    content JSONB NOT NULL,
    published_by BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT fk_mail_template_version_template FOREIGN KEY (template_pk) REFERENCES t_mail_template (id) ON DELETE CASCADE,
    CONSTRAINT uk_mail_template_version UNIQUE (template_pk, version),
    CONSTRAINT ck_mail_template_version CHECK (version > 0)
);

COMMENT ON COLUMN t_mail_template_version.content IS 'Immutable publication snapshot; restoring it creates a new draft.';

COMMENT ON COLUMN t_mail_template_version.published_by IS 'Publisher user ID; zero identifies seed data.';

-- +migrate Down
DROP TABLE t_mail_template_version;

DROP TABLE t_mail_template;

DROP TRIGGER auth_email_after_insert ON t_auth_email_code;

DROP FUNCTION auth_email_after_insert();

DROP FUNCTION auth_enqueue_email(TEXT, TEXT);

DELETE FROM river_job WHERE kind IN ('auth_email_deliver', 'auth_sensitive_activity_scan', 'mail_template_test_deliver', 'auth_history_cleanup');

DROP TABLE t_sensitive_scan;

DROP TABLE t_sensitive_operation;

DROP TABLE t_auth_email_cooldown;

DROP TABLE t_auth_email_code;

DROP TRIGGER auth_email_alias_check ON t_user;

DROP FUNCTION auth_email_alias_check();

DROP TABLE t_audit_event;

DROP TABLE t_user_msg_status;

DROP TABLE t_msg_recipient;

DROP TABLE t_msg_send_receipt;

DROP FUNCTION msg_send_receipt_withdrawn();

DROP TABLE t_msg;

DROP TABLE t_dictionary;

DROP TABLE t_user_user_group_relation;

DROP TABLE t_user_recovery_code;

DROP TABLE t_user_totp;

DROP TABLE t_user;

DROP TABLE t_user_group;

DROP TABLE t_role;

DROP TABLE t_action;

DROP FUNCTION action_resource_bindings_valid(JSONB, TEXT);
