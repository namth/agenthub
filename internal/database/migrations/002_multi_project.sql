-- FEAT-04 Đa dự án (DEC-14 → DEC-17)
ALTER TABLE projects ADD COLUMN weight INTEGER NOT NULL DEFAULT 1 CHECK(weight BETWEEN 1 AND 5);
ALTER TABLE projects ADD COLUMN max_parallel INTEGER CHECK(max_parallel BETWEEN 1 AND 50);
ALTER TABLE projects ADD COLUMN webhook_id INTEGER;

CREATE TABLE IF NOT EXISTS project_members (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK(role IN ('ADMIN', 'PARTNER')),
    synced_at DATETIME,
    PRIMARY KEY (project_id, user_id)
);

-- Agent không có dòng nào = nhận mọi dự án.
CREATE TABLE IF NOT EXISTS agent_projects (
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_id, project_id)
);

CREATE INDEX IF NOT EXISTS idx_jobs_project_status ON jobs(project_id, status);
