ALTER TABLE agents ADD COLUMN auth_status TEXT NOT NULL DEFAULT 'UNKNOWN'
    CHECK(auth_status IN ('OK', 'NEEDS_AUTH', 'UNKNOWN'));
ALTER TABLE agents ADD COLUMN auth_checked_at DATETIME;
ALTER TABLE agents ADD COLUMN auth_detail TEXT;
ALTER TABLE workers ADD COLUMN gh_auth TEXT NOT NULL DEFAULT 'UNKNOWN'
    CHECK(gh_auth IN ('OK', 'NEEDS_AUTH', 'UNKNOWN'));
ALTER TABLE users ADD COLUMN last_seen_at DATETIME;
CREATE INDEX IF NOT EXISTS idx_agents_auth ON agents(auth_status);

-- DEC-29: mặc định chỉ Claude trên máy owner được làm lead
UPDATE agents SET lead_eligible = 0 WHERE kind <> 'CLAUDE';
