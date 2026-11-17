-- 000001_init
-- Initial schema for the feature flag platform.
--
-- Design notes:
--   * UUID primary keys throughout, generated with gen_random_uuid() (pgcrypto).
--   * Same-project integrity for flag states and SDK keys is enforced by
--     COMPOSITE FOREIGN KEYS, so a cross-project row is not representable
--     rather than merely unchecked in application code.
--   * This migration runs as ff_migrator, which owns the public schema.
--   * The ff_runtime role receives DML on the resulting tables through
--     default privileges declared FOR ROLE ff_migrator.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- users
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text        NOT NULL,
    password_hash text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Email is stored normalized (lowercase, trimmed) by the application.
CREATE UNIQUE INDEX users_email_key ON users (email);

-- ---------------------------------------------------------------------------
-- sessions
-- ---------------------------------------------------------------------------
CREATE TABLE sessions (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- SHA-256 of the opaque session token. The raw token is never stored.
    token_hash   text        NOT NULL,
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX sessions_token_hash_key ON sessions (token_hash);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- ---------------------------------------------------------------------------
-- projects
-- ---------------------------------------------------------------------------
CREATE TABLE projects (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Explicit tenant boundary. Every project-scoped authorization check
    -- compares this against the session's user.
    owner_id   uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    name       text        NOT NULL,
    slug       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX projects_slug_key ON projects (slug);
CREATE UNIQUE INDEX projects_id_owner_id_key ON projects (id, owner_id);

-- ---------------------------------------------------------------------------
-- environments
-- ---------------------------------------------------------------------------
CREATE TABLE environments (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id     uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    -- One of development, staging, production. Immutable.
    key            text        NOT NULL,
    name           text        NOT NULL,
    -- Monotonic version of this environment's complete flag configuration.
    config_version bigint      NOT NULL DEFAULT 1,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT environments_key_check CHECK (key IN ('development', 'staging', 'production')),
    CONSTRAINT environments_config_version_check CHECK (config_version >= 1)
);

CREATE UNIQUE INDEX environments_project_id_key_key ON environments (project_id, key);
-- Required as the target of the composite foreign keys below.
CREATE UNIQUE INDEX environments_id_project_id_key ON environments (id, project_id);
CREATE INDEX environments_project_id_idx ON environments (project_id);

-- ---------------------------------------------------------------------------
-- feature_flags
-- ---------------------------------------------------------------------------
CREATE TABLE feature_flags (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    key         text        NOT NULL,
    description text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    -- Conservative key pattern: lowercase letters, digits, underscore, hyphen.
    CONSTRAINT feature_flags_key_check
        CHECK (key ~ '^[a-z0-9][a-z0-9_-]{0,63}$')
);

CREATE UNIQUE INDEX feature_flags_project_id_key_key ON feature_flags (project_id, key);
-- Required as the target of the composite foreign key below.
CREATE UNIQUE INDEX feature_flags_id_project_id_key ON feature_flags (id, project_id);
CREATE INDEX feature_flags_project_id_idx ON feature_flags (project_id);

-- ---------------------------------------------------------------------------
-- feature_flag_states
-- ---------------------------------------------------------------------------
CREATE TABLE feature_flag_states (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    flag_id        uuid        NOT NULL,
    environment_id uuid        NOT NULL,
    -- Denormalized join key that makes same-project pairing expressible.
    project_id     uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    enabled        boolean     NOT NULL DEFAULT false,
    updated_at     timestamptz NOT NULL DEFAULT now(),

    -- A flag and an environment must belong to the same project. These
    -- composite foreign keys make a cross-project row unrepresentable.
    CONSTRAINT feature_flag_states_flag_fk
        FOREIGN KEY (flag_id, project_id)
        REFERENCES feature_flags (id, project_id) ON DELETE CASCADE,
    CONSTRAINT feature_flag_states_environment_fk
        FOREIGN KEY (environment_id, project_id)
        REFERENCES environments (id, project_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX feature_flag_states_flag_id_environment_id_key
    ON feature_flag_states (flag_id, environment_id);
CREATE INDEX feature_flag_states_environment_id_idx ON feature_flag_states (environment_id);
CREATE INDEX feature_flag_states_flag_id_idx ON feature_flag_states (flag_id);
CREATE INDEX feature_flag_states_project_id_environment_id_idx
    ON feature_flag_states (project_id, environment_id);

-- ---------------------------------------------------------------------------
-- sdk_keys
-- ---------------------------------------------------------------------------
CREATE TABLE sdk_keys (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id     uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    environment_id uuid        NOT NULL,
    name           text        NOT NULL,
    -- Safe display identifier. Never used for lookup or authentication.
    key_prefix     text        NOT NULL,
    -- SHA-256 (optionally HMAC-SHA-256 with a server-side pepper) of the
    -- key's secret portion. The raw key is never stored.
    key_hash       text        NOT NULL,
    key_type       text        NOT NULL,
    revoked_at     timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT sdk_keys_key_type_check CHECK (key_type IN ('client', 'server')),
    -- An SDK key's environment must belong to the key's project.
    CONSTRAINT sdk_keys_environment_fk
        FOREIGN KEY (environment_id, project_id)
        REFERENCES environments (id, project_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX sdk_keys_key_hash_key ON sdk_keys (key_hash);
CREATE INDEX sdk_keys_environment_id_idx ON sdk_keys (environment_id);
CREATE INDEX sdk_keys_project_id_idx ON sdk_keys (project_id);
-- Partial index for listing active keys.
CREATE INDEX sdk_keys_active_idx ON sdk_keys (environment_id) WHERE revoked_at IS NULL;
