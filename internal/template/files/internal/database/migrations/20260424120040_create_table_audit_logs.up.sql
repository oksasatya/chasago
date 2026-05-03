CREATE TABLE audit_logs (
    id           UUID        PRIMARY KEY,
    user_id      UUID        REFERENCES users(id) ON DELETE SET NULL,
    action       TEXT        NOT NULL,
    resource     TEXT        NOT NULL DEFAULT '',
    resource_id  TEXT,
    metadata     JSONB,
    ip           TEXT,
    user_agent   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_logs_user_id    ON audit_logs (user_id);
CREATE INDEX idx_audit_logs_action     ON audit_logs (action);
CREATE INDEX idx_audit_logs_created_at ON audit_logs (created_at DESC);
