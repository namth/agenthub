-- FEAT-05 Thêm máy bằng một lệnh, máy của partner (DEC-18 → DEC-20)
ALTER TABLE workers ADD COLUMN owner_user_id TEXT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE workers ADD COLUMN version TEXT;
ALTER TABLE workers ADD COLUMN paused INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN lead_eligible INTEGER NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS join_codes (
    code_hash TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    worker_name TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
