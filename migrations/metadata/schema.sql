-- SIL canonical metadata schema.
-- Update this snapshot whenever a durable metadata change is introduced.

CREATE TABLE IF NOT EXISTS identities (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    display_name TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS identity_identifiers (
    identity_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    identifier_type TEXT NOT NULL,
    identifier_value TEXT NOT NULL,
    PRIMARY KEY (tenant_id, identifier_type, identifier_value),
    FOREIGN KEY (identity_id) REFERENCES identities (id)
);

CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    channel TEXT NOT NULL,
    case_id TEXT,
    customer_identity_id TEXT,
    subject TEXT,
    status TEXT NOT NULL,
    last_message_id TEXT,
    started_at TIMESTAMP NOT NULL,
    last_activity_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS interactions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    channel TEXT NOT NULL,
    provider TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    case_id TEXT,
    customer_identity_id TEXT,
    direction TEXT NOT NULL,
    subject TEXT,
    body TEXT,
    occurred_at TIMESTAMP NOT NULL,
    received_at TIMESTAMP NOT NULL,
    payload JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS cases (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    customer_identity_id TEXT,
    primary_channel TEXT,
    subject TEXT NOT NULL,
    summary TEXT,
    status TEXT NOT NULL,
    priority TEXT NOT NULL,
    opened_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS case_evidence_links (
    case_id TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    source_id TEXT NOT NULL,
    PRIMARY KEY (case_id, source_kind, source_id),
    FOREIGN KEY (case_id) REFERENCES cases (id)
);

CREATE TABLE IF NOT EXISTS agent_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    case_id TEXT,
    session_id TEXT NOT NULL,
    goal TEXT NOT NULL,
    status TEXT NOT NULL,
    runtime_name TEXT,
    planner_name TEXT,
    started_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_steps (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    summary TEXT NOT NULL,
    tool_name TEXT,
    status TEXT NOT NULL,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    FOREIGN KEY (run_id) REFERENCES agent_runs (id)
);

CREATE TABLE IF NOT EXISTS tool_calls (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    step_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    status TEXT NOT NULL,
    target_tenant_id TEXT,
    attempt INTEGER NOT NULL DEFAULT 1,
    latency_ms INTEGER,
    error_code TEXT,
    error_message TEXT,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    FOREIGN KEY (run_id) REFERENCES agent_runs (id),
    FOREIGN KEY (step_id) REFERENCES agent_steps (id)
);

CREATE TABLE IF NOT EXISTS retrieval_events (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    step_id TEXT,
    tenant_id TEXT NOT NULL,
    query_text TEXT NOT NULL,
    hit_count INTEGER NOT NULL,
    top_score REAL,
    degraded BOOLEAN NOT NULL DEFAULT FALSE,
    occurred_at TIMESTAMP NOT NULL,
    FOREIGN KEY (run_id) REFERENCES agent_runs (id),
    FOREIGN KEY (step_id) REFERENCES agent_steps (id)
);

CREATE TABLE IF NOT EXISTS quality_signals (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    step_id TEXT,
    tenant_id TEXT NOT NULL,
    signal_type TEXT NOT NULL,
    severity TEXT NOT NULL,
    score REAL,
    description TEXT NOT NULL,
    occurred_at TIMESTAMP NOT NULL,
    FOREIGN KEY (run_id) REFERENCES agent_runs (id),
    FOREIGN KEY (step_id) REFERENCES agent_steps (id)
);

CREATE TABLE IF NOT EXISTS approval_events (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    step_id TEXT,
    tool_call_id TEXT,
    tenant_id TEXT NOT NULL,
    action_name TEXT NOT NULL,
    decision TEXT NOT NULL,
    requested_by TEXT,
    decided_by TEXT,
    reason TEXT,
    occurred_at TIMESTAMP NOT NULL,
    FOREIGN KEY (run_id) REFERENCES agent_runs (id),
    FOREIGN KEY (step_id) REFERENCES agent_steps (id),
    FOREIGN KEY (tool_call_id) REFERENCES tool_calls (id)
);
