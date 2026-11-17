-- 000001_init (down)
-- Reverse order to respect foreign key dependencies.

DROP TABLE IF EXISTS sdk_keys;
DROP TABLE IF EXISTS feature_flag_states;
DROP TABLE IF EXISTS feature_flags;
DROP TABLE IF EXISTS environments;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS users;

-- pgcrypto is intentionally left installed. It is a Supabase-provided
-- extension that other parts of the platform may depend on, and dropping
-- it is not necessary to reverse this migration.
