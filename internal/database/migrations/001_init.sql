-- Bảng USERS
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    github_id TEXT UNIQUE NOT NULL,
    username TEXT NOT NULL,
    avatar_url TEXT,
    role TEXT NOT NULL CHECK(role IN ('ADMIN', 'PARTNER')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Bảng PROJECTS
CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    github_repo TEXT NOT NULL UNIQUE,
    default_branch TEXT NOT NULL DEFAULT 'main',
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK(status IN ('ACTIVE', 'ARCHIVED')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Bảng WORKERS
CREATE TABLE IF NOT EXISTS workers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    os TEXT NOT NULL,
    conn_status TEXT NOT NULL DEFAULT 'OFFLINE' CHECK(conn_status IN ('ONLINE', 'OFFLINE')),
    offline_since DATETIME,
    token_hash TEXT NOT NULL,
    last_heartbeat DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Bảng AGENTS
CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    worker_id TEXT NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('CLAUDE', 'ANTIGRAVITY')),
    status TEXT NOT NULL DEFAULT 'IDLE' CHECK(status IN ('IDLE', 'BUSY', 'COOLDOWN')),
    cooldown_until DATETIME,
    cooldown_reason TEXT CHECK(cooldown_reason IN ('EXHAUSTED', 'SOFT_LIMIT')),
    lead_priority INTEGER CHECK(lead_priority BETWEEN 1 AND 99),
    soft_max_jobs INTEGER CHECK(soft_max_jobs BETWEEN 1 AND 500),
    soft_window_hours INTEGER NOT NULL DEFAULT 5 CHECK(soft_window_hours BETWEEN 1 AND 24),
    idle_since DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(worker_id, kind)
);

CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(kind, status);

-- Bảng TASKS
CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_id TEXT REFERENCES tasks(id) ON DELETE CASCADE,
    github_issue_id INTEGER,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING_APPROVAL' 
        CHECK(status IN ('PENDING_APPROVAL', 'PLANNING', 'READY', 'IN_PROGRESS', 'WAITING_QA', 'QA_PASSED', 'DONE', 'REJECTED')),
    assigned_worker_id TEXT REFERENCES workers(id) ON DELETE SET NULL,
    branch_name TEXT,
    pr_url TEXT,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_worker ON tasks(assigned_worker_id);

-- Bảng JOBS
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('SPEC','DESIGN','TEST_WRITE','CODE','QA_REVIEW','REPLAN','UNBLOCK')),
    required_agent TEXT NOT NULL CHECK(required_agent IN ('CLAUDE', 'ANTIGRAVITY')),
    lead_only INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'QUEUED' CHECK(status IN ('QUEUED','RUNNING','DONE','FAILED','CANCELLED')),
    priority INTEGER NOT NULL DEFAULT 100,
    attempt INTEGER NOT NULL DEFAULT 0,
    branch_name TEXT,
    current_assignment_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_jobs_queue ON jobs(status, required_agent, priority, created_at);

-- Bảng ASSIGNMENTS
CREATE TABLE IF NOT EXISTS assignments (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id),
    attempt INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE'
        CHECK(status IN ('ACTIVE','SUBMITTED','EXHAUSTED','RECLAIMED','ABORTED','FAILED')),
    handoff_note TEXT,
    wip_commit TEXT,
    last_log_at DATETIME,
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    ended_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_assignments_agent ON assignments(agent_id, started_at);

-- Bảng LEAD_LEASE
CREATE TABLE IF NOT EXISTS lead_lease (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    state TEXT NOT NULL DEFAULT 'VACANT' CHECK(state IN ('HELD', 'VACANT')),
    agent_id TEXT REFERENCES agents(id) ON DELETE SET NULL,
    epoch INTEGER NOT NULL DEFAULT 0,
    expires_at DATETIME,
    vacant_since DATETIME
);

INSERT OR IGNORE INTO lead_lease (id, state, epoch) VALUES (1, 'VACANT', 0);

-- Bảng LEAD_JOURNAL
CREATE TABLE IF NOT EXISTS lead_journal (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epoch INTEGER NOT NULL,
    agent_id TEXT,
    task_id TEXT REFERENCES tasks(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('HANDOVER','DECISION','NOTE','ACTION')),
    entry TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_lead_journal_task ON lead_journal(task_id, created_at);

-- Bảng TASK_LOGS
CREATE TABLE IF NOT EXISTS task_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    worker_id TEXT NOT NULL REFERENCES workers(id),
    assignment_id TEXT REFERENCES assignments(id),
    log_chunk TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_task_logs_task ON task_logs(task_id);

-- Bảng SETTINGS
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO settings (key, value) VALUES ('office_enabled', 'false');
