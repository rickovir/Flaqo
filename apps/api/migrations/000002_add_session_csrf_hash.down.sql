-- 000002_add_session_csrf_hash (down)

ALTER TABLE sessions DROP COLUMN IF EXISTS csrf_token_hash;
