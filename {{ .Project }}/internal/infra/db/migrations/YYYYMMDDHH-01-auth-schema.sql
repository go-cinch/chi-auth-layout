-- +migrate Up
CREATE TABLE t_action (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    name VARCHAR(50) NOT NULL,
    action_group VARCHAR(50) NOT NULL,
    code CHAR(8) NOT NULL,
    word VARCHAR(50) NOT NULL,
    resource TEXT NOT NULL DEFAULT '',
    menu TEXT NOT NULL DEFAULT '',
    btn TEXT NOT NULL DEFAULT '',
    CONSTRAINT uk_action_code UNIQUE (code),
    CONSTRAINT uk_action_word UNIQUE (word),
    CONSTRAINT ck_action_name CHECK (CHAR_LENGTH(TRIM(name)) > 0),
    CONSTRAINT ck_action_group CHECK (CHAR_LENGTH(TRIM(action_group)) > 0),
    CONSTRAINT ck_action_code CHECK (CHAR_LENGTH(TRIM(code)) = 8),
    CONSTRAINT ck_action_word CHECK (CHAR_LENGTH(TRIM(word)) > 0)
);

COMMENT ON TABLE t_action IS 'permission actions';
COMMENT ON COLUMN t_action.action_group IS 'display and search group for related actions';
COMMENT ON COLUMN t_action.code IS 'stable eight-character action code';
COMMENT ON COLUMN t_action.word IS 'unique action keyword used by clients';
COMMENT ON COLUMN t_action.resource IS 'newline-separated resource permission rules';
COMMENT ON COLUMN t_action.menu IS 'newline-separated permitted menu paths';
COMMENT ON COLUMN t_action.btn IS 'newline-separated permitted button codes';

CREATE TABLE t_role (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    name VARCHAR(50) NOT NULL,
    word VARCHAR(50) NOT NULL,
    action TEXT NOT NULL DEFAULT '',
    CONSTRAINT uk_role_word UNIQUE (word),
    CONSTRAINT ck_role_name CHECK (CHAR_LENGTH(TRIM(name)) > 0),
    CONSTRAINT ck_role_word CHECK (CHAR_LENGTH(TRIM(word)) > 0)
);

COMMENT ON TABLE t_role IS 'user roles';
COMMENT ON COLUMN t_role.word IS 'unique role keyword used by clients';
COMMENT ON COLUMN t_role.action IS 'comma-separated action codes';

CREATE TABLE t_user_group (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    name VARCHAR(50) NOT NULL,
    word VARCHAR(50) NOT NULL,
    action TEXT NOT NULL DEFAULT '',
    CONSTRAINT uk_user_group_word UNIQUE (word),
    CONSTRAINT ck_user_group_name CHECK (CHAR_LENGTH(TRIM(name)) > 0),
    CONSTRAINT ck_user_group_word CHECK (CHAR_LENGTH(TRIM(word)) > 0)
);

COMMENT ON TABLE t_user_group IS 'user permission groups';
COMMENT ON COLUMN t_user_group.word IS 'unique group keyword used by clients';
COMMENT ON COLUMN t_user_group.action IS 'comma-separated action codes';

CREATE TABLE t_user (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    role_id BIGINT NULL,
    action TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL,
    code CHAR(8) NOT NULL,
    password TEXT NOT NULL,
    last_logged_in_at TIMESTAMP NULL,
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
COMMENT ON COLUMN t_user.role_id IS 'optional role id';
COMMENT ON COLUMN t_user.action IS 'comma-separated user-specific action codes';
COMMENT ON COLUMN t_user.username IS 'unique login name';
COMMENT ON COLUMN t_user.code IS 'stable eight-character user code';
COMMENT ON COLUMN t_user.password IS 'adaptive password hash';
COMMENT ON COLUMN t_user.last_logged_in_at IS 'last successful login time';
COMMENT ON COLUMN t_user.status IS 'account state: 0 pending approval, 1 active, 2 locked';
COMMENT ON COLUMN t_user.metadata IS 'extensible user attributes; lock_expired_at is Unix milliseconds and zero means a permanent lock; password_change_failures disables self-service password changes at the configured threshold';
COMMENT ON COLUMN t_user.wrong IS 'consecutive wrong-password count';
COMMENT ON COLUMN t_user.credential_version IS 'Incremented atomically on password changes to invalidate all previously issued sessions';
COMMENT ON COLUMN t_user.login_count IS 'Completed logins since the last administrator password reset; zero requires password reset before business access';

CREATE INDEX idx_user_role_id ON t_user (role_id);

CREATE TABLE t_user_user_group_relation (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id BIGINT NOT NULL,
    user_group_id BIGINT NOT NULL,
    CONSTRAINT uk_user_user_group_relation UNIQUE (user_id, user_group_id),
    CONSTRAINT fk_user_user_group_relation_user FOREIGN KEY (user_id) REFERENCES t_user (id) ON DELETE CASCADE,
    CONSTRAINT fk_user_user_group_relation_group FOREIGN KEY (user_group_id) REFERENCES t_user_group (id) ON DELETE CASCADE
);

COMMENT ON TABLE t_user_user_group_relation IS 'many-to-many relation between users and user groups';
COMMENT ON COLUMN t_user_user_group_relation.user_id IS 'related user id';
COMMENT ON COLUMN t_user_user_group_relation.user_group_id IS 'related user group id';

CREATE INDEX idx_user_user_group_relation_user_group_id
    ON t_user_user_group_relation (user_group_id);

CREATE TABLE t_whitelist (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    category SMALLINT NOT NULL,
    resource TEXT NOT NULL,
    CONSTRAINT uk_whitelist_category_resource UNIQUE (category, resource),
    CONSTRAINT ck_whitelist_category CHECK (category IN (0, 1)),
    CONSTRAINT ck_whitelist_resource CHECK (CHAR_LENGTH(TRIM(resource)) > 0)
);

COMMENT ON TABLE t_whitelist IS 'permission and JWT bypass rules';
COMMENT ON COLUMN t_whitelist.category IS 'rule category: 0 permission, 1 JWT';
COMMENT ON COLUMN t_whitelist.resource IS 'newline-separated resource rules';

CREATE INDEX idx_whitelist_category ON t_whitelist (category);

CREATE TABLE t_dictionary (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
COMMENT ON COLUMN t_dictionary.dictionary_key IS 'stable uppercase key with underscore-separated segments';
COMMENT ON COLUMN t_dictionary.value IS 'arbitrary JSON dictionary value';
COMMENT ON COLUMN t_dictionary.description IS 'administrator-facing purpose and usage notes';
COMMENT ON COLUMN t_dictionary.enabled IS 'whether runtime consumers may read this dictionary';

-- +migrate Down
DROP TABLE t_dictionary;
DROP TABLE t_whitelist;
DROP TABLE t_user_user_group_relation;
DROP TABLE t_user;
DROP TABLE t_user_group;
DROP TABLE t_role;
DROP TABLE t_action;
