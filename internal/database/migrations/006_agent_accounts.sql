-- agenthub:foreign_keys=off

-- 1. Dựng lại bảng agents với name, ever_ok, removed_at và UNIQUE(worker_id, name)
CREATE TABLE agents_new (
    id TEXT PRIMARY KEY,
    worker_id TEXT NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('CLAUDE', 'ANTIGRAVITY')),
    name TEXT,
    status TEXT NOT NULL DEFAULT 'IDLE' CHECK(status IN ('IDLE', 'BUSY', 'COOLDOWN')),
    cooldown_until DATETIME,
    cooldown_reason TEXT CHECK(cooldown_reason IN ('EXHAUSTED', 'SOFT_LIMIT')),
    lead_priority INTEGER CHECK(lead_priority BETWEEN 1 AND 99),
    soft_max_jobs INTEGER CHECK(soft_max_jobs BETWEEN 1 AND 500),
    soft_window_hours INTEGER NOT NULL DEFAULT 5 CHECK(soft_window_hours BETWEEN 1 AND 24),
    idle_since DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    lead_eligible INTEGER NOT NULL DEFAULT 1,
    auth_status TEXT NOT NULL DEFAULT 'UNKNOWN' CHECK(auth_status IN ('OK', 'NEEDS_AUTH', 'UNKNOWN')),
    auth_checked_at DATETIME,
    auth_detail TEXT,
    ever_ok INTEGER NOT NULL DEFAULT 1,
    removed_at DATETIME,
    UNIQUE(worker_id, name)
);

INSERT INTO agents_new (
    id, worker_id, kind, name, status, cooldown_until, cooldown_reason,
    lead_priority, soft_max_jobs, soft_window_hours, idle_since, created_at,
    lead_eligible, auth_status, auth_checked_at, auth_detail, ever_ok, removed_at
)
SELECT
    id,
    worker_id,
    kind,
    substr(id, instr(id, '/') + 1) AS name,
    status,
    cooldown_until,
    cooldown_reason,
    lead_priority,
    soft_max_jobs,
    soft_window_hours,
    idle_since,
    created_at,
    lead_eligible,
    auth_status,
    auth_checked_at,
    auth_detail,
    CASE
        WHEN auth_status <> 'OK' AND NOT EXISTS (SELECT 1 FROM assignments WHERE agent_id = agents.id) THEN 0
        ELSE 1
    END AS ever_ok,
    NULL AS removed_at
FROM agents;

DROP TABLE agents;

ALTER TABLE agents_new RENAME TO agents;

CREATE TRIGGER IF NOT EXISTS trg_agents_default_name
AFTER INSERT ON agents
WHEN NEW.name IS NULL
BEGIN
    UPDATE agents
    SET name = substr(NEW.id, instr(NEW.id, '/') + 1)
    WHERE id = NEW.id;
END;

CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(kind, status);
CREATE INDEX IF NOT EXISTS idx_agents_auth ON agents(auth_status);
CREATE INDEX IF NOT EXISTS idx_agents_worker ON agents(worker_id);
