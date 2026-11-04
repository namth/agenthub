-- agenthub:foreign_keys=off

-- 1. Dựng lại bảng tasks để CHECK(status) có thêm 'WAITING_USER'
CREATE TABLE tasks_new (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_id TEXT REFERENCES tasks(id) ON DELETE CASCADE,
    github_issue_id INTEGER,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING_APPROVAL' 
        CHECK(status IN ('PENDING_APPROVAL', 'PLANNING', 'WAITING_USER', 'READY', 'IN_PROGRESS', 'WAITING_QA', 'QA_PASSED', 'DONE', 'REJECTED')),
    assigned_worker_id TEXT REFERENCES workers(id) ON DELETE SET NULL,
    branch_name TEXT,
    pr_url TEXT,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO tasks_new (id, project_id, parent_id, github_issue_id, title, description, status, assigned_worker_id, branch_name, pr_url, created_by, created_at, updated_at)
    SELECT id, project_id, parent_id, github_issue_id, title, description, status, assigned_worker_id, branch_name, pr_url, created_by, created_at, updated_at FROM tasks;

DROP TABLE tasks;

ALTER TABLE tasks_new RENAME TO tasks;

CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_worker ON tasks(assigned_worker_id);

-- 2. FEAT-07 & FEAT-08: Vòng lặp phỏng vấn làm rõ
CREATE TABLE IF NOT EXISTS task_clarifications (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    round INTEGER NOT NULL,
    questions TEXT NOT NULL,
    answers TEXT,
    status TEXT NOT NULL DEFAULT 'WAITING' CHECK(status IN ('WAITING','ANSWERED','AUTO_DECIDED')),
    answered_by TEXT REFERENCES users(id),
    answered_via TEXT CHECK(answered_via IN ('WEB','GITHUB','TELEGRAM','MCP','AUTO')),
    asked_by_agent TEXT,
    reminded_at DATETIME,
    answered_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(task_id, round)
);

CREATE INDEX IF NOT EXISTS idx_clarifications_task ON task_clarifications(task_id, status);

-- 3. FEAT-09: Vận hành thật & an toàn đa partner
ALTER TABLE projects ADD COLUMN allowed_commands TEXT;
ALTER TABLE projects ADD COLUMN needs_ci_for_integration INTEGER NOT NULL DEFAULT 0;
ALTER TABLE projects ADD COLUMN is_sandbox INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS smoke_runs (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK(status IN ('RUNNING','PASSED','FAILED','TIMEOUT')),
    steps TEXT NOT NULL DEFAULT '[]',
    started_by TEXT REFERENCES users(id),
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    ended_at DATETIME
);

-- 4. FEAT-10: Điều hành mọi nơi
CREATE TABLE IF NOT EXISTS personal_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    last4 TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    last_used_at DATETIME,
    revoked_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);

CREATE TABLE IF NOT EXISTS lead_proposals (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('CANCEL')),
    reason TEXT NOT NULL,
    proposed_by TEXT NOT NULL,
    epoch INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','ACCEPTED','REJECTED')),
    decided_by TEXT REFERENCES users(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    decided_at DATETIME
);
