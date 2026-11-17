-- 000002_add_session_csrf_hash
--
-- Bind a CSRF token to each session.
--
-- The token is stored as a hash, never in the clear, so a database read
-- does not yield a usable token. The client keeps its own copy and sends
-- it in the X-CSRF-Token header; the server compares the hash of the
-- submitted token against this value.

ALTER TABLE sessions
    ADD COLUMN csrf_token_hash text;

-- Existing sessions predate CSRF binding and cannot be used for
-- state-changing requests, so they are removed rather than left in a
-- partially valid state.
DELETE FROM sessions WHERE csrf_token_hash IS NULL;
