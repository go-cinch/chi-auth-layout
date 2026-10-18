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
                AND binding ->> 'operation' IN ('read', 'create', 'update', 'delete')
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
        CASE WHEN config ? 'builtin' THEN
            JSONB_TYPEOF(config -> 'builtin') = 'string'
            AND config ->> 'builtin' IN ('all', 'default')
            AND config ->> 'builtin' = SPLIT_PART(word, ':', 2)
        ELSE
            SPLIT_PART(word, ':', 2) NOT IN ('all', 'default')
        END
    ),
    CONSTRAINT ck_action_all_config CHECK (
        config ->> 'builtin' IS DISTINCT FROM 'all'
        OR (config -> 'ui' = '[]'::jsonb AND config -> 'endpoints' = '[]'::jsonb)
    ),
    CONSTRAINT ck_action_resource_binding CHECK ((
        NOT (config ? 'resource_key') AND NOT (config ? 'operation')
        AND CASE
            WHEN config ->> 'builtin' IN ('all', 'default')
                OR word = scope || ':grant:system:permission-admin'
            THEN NOT (config ? 'bindings')
            ELSE action_resource_bindings_valid(config -> 'bindings', scope)
        END
    ) IS TRUE)
);

COMMENT ON TABLE t_action IS 'Scoped permission catalog with UI and transport mappings; labels reside in I18N_CATALOG';

COMMENT ON COLUMN t_action.created_at IS 'Row creation time in Unix milliseconds';

COMMENT ON COLUMN t_action.updated_at IS 'Last permission update time in Unix milliseconds';

COMMENT ON COLUMN t_action.action_group IS 'Stable scope:module:resource group identifier; label lookup uses frontend.groups[action_group]';

COMMENT ON COLUMN t_action.code IS 'Stable eight-character assignment code used by users, roles and user groups';

COMMENT ON COLUMN t_action.word IS 'Stable scope:operation:module:resource permission identifier; label lookup uses frontend.actions[word]';

COMMENT ON COLUMN t_action.scope IS 'Stored scope derived from word; label lookup uses frontend.scopes[scope]';

COMMENT ON COLUMN t_action.config IS 'Explicit resource/operation bindings select logical feature keys; ui and endpoints are derived projections. builtin=all grants the scope; builtin=default selects explicit resources and features';

CREATE INDEX idx_action_scope_group ON t_action (scope, action_group);

CREATE INDEX idx_action_config ON t_action USING GIN (config jsonb_path_ops);

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
    code CHAR(8) NOT NULL,
    password TEXT NOT NULL,
    last_logged_in_at BIGINT NULL,
    status SMALLINT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    wrong BIGINT NOT NULL DEFAULT 0,
    credential_version BIGINT NOT NULL DEFAULT 1,
    login_count BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT uk_user_username UNIQUE (username),
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
    CONSTRAINT ck_user_wrong CHECK (wrong >= 0)
);

COMMENT ON TABLE t_user IS 'authentication users';

COMMENT ON COLUMN t_user.created_at IS 'Account creation time in Unix milliseconds';

COMMENT ON COLUMN t_user.updated_at IS 'Last account update time in Unix milliseconds';

COMMENT ON COLUMN t_user.role_id IS 'optional role id';

COMMENT ON COLUMN t_user.action IS 'comma-separated user-specific action codes';

COMMENT ON COLUMN t_user.username IS 'unique login name';

COMMENT ON COLUMN t_user.code IS 'stable eight-character user code';

COMMENT ON COLUMN t_user.password IS 'adaptive password hash';

COMMENT ON COLUMN t_user.last_logged_in_at IS 'Last successful login time in Unix milliseconds; null before the first successful login';

COMMENT ON COLUMN t_user.status IS 'account state: 0 pending approval, 1 active, 2 locked';

COMMENT ON COLUMN t_user.metadata IS 'extensible user attributes; lock_expired_at is Unix milliseconds and zero means a permanent lock; password_change_failures disables self-service password changes at the configured threshold';

COMMENT ON COLUMN t_user.wrong IS 'consecutive wrong-password count';

COMMENT ON COLUMN t_user.credential_version IS 'Incremented atomically on password changes to invalidate all previously issued sessions';

COMMENT ON COLUMN t_user.login_count IS 'Completed logins since the last administrator password reset; zero requires password reset before business access';

CREATE INDEX idx_user_role_id ON t_user (role_id);

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

CREATE INDEX idx_user_user_group_relation_user_group_id
    ON t_user_user_group_relation (user_group_id);

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

CREATE INDEX idx_audit_event_kind_time ON t_audit_event (kind, occurred_at DESC, id DESC);

CREATE INDEX idx_audit_event_username_time ON t_audit_event (username, occurred_at DESC);

-- +migrate Down
DROP TABLE t_audit_event;

DROP TABLE t_user_msg_status;

DROP TABLE t_msg_recipient;

DROP TABLE t_msg;

DROP TABLE t_dictionary;

DROP TABLE t_user_user_group_relation;

DROP TABLE t_user;

DROP TABLE t_user_group;

DROP TABLE t_role;

DROP TABLE t_action;

DROP FUNCTION action_resource_bindings_valid(JSONB, TEXT);
