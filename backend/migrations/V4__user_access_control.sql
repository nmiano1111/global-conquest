ALTER TABLE users
  ADD COLUMN IF NOT EXISTS access_status text NOT NULL DEFAULT 'active';

ALTER TABLE users
  ADD CONSTRAINT users_access_status_ck
  CHECK (access_status IN ('active', 'blocked'));
