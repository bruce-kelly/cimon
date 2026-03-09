CREATE TABLE IF NOT EXISTS workflow_runs (
    id INTEGER PRIMARY KEY,
    repo TEXT NOT NULL,
    name TEXT NOT NULL,
    workflow_file TEXT NOT NULL,
    head_branch TEXT,
    head_sha TEXT,
    status TEXT,
    conclusion TEXT,
    event TEXT,
    actor TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    html_url TEXT
);

CREATE TABLE IF NOT EXISTS pull_requests (
    repo TEXT NOT NULL,
    number INTEGER NOT NULL,
    title TEXT,
    author TEXT,
    state TEXT,
    draft INTEGER DEFAULT 0,
    created_at TEXT,
    updated_at TEXT,
    ci_status TEXT,
    review_state TEXT,
    is_agent INTEGER DEFAULT 0,
    html_url TEXT,
    PRIMARY KEY (repo, number)
);

CREATE TABLE IF NOT EXISTS jobs (
    id INTEGER PRIMARY KEY,
    run_id INTEGER NOT NULL,
    repo TEXT NOT NULL,
    name TEXT NOT NULL,
    conclusion TEXT,
    started_at TEXT,
    completed_at TEXT,
    FOREIGN KEY (run_id) REFERENCES workflow_runs(id)
);

CREATE TABLE IF NOT EXISTS agent_tasks (
    id TEXT PRIMARY KEY,
    repo TEXT NOT NULL,
    task TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'running',
    started_at TEXT NOT NULL,
    completed_at TEXT,
    exit_code INTEGER,
    pr_number INTEGER
);

CREATE TABLE IF NOT EXISTS review_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo TEXT NOT NULL,
    pr_number INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS dismissed_items (
    repo TEXT NOT NULL,
    number INTEGER NOT NULL,
    dismissed_at TEXT NOT NULL,
    PRIMARY KEY (repo, number)
);

CREATE INDEX IF NOT EXISTS idx_runs_repo ON workflow_runs(repo);
CREATE INDEX IF NOT EXISTS idx_runs_updated ON workflow_runs(updated_at);
CREATE INDEX IF NOT EXISTS idx_jobs_run ON jobs(run_id);
CREATE INDEX IF NOT EXISTS idx_jobs_repo_name ON jobs(repo, name);
CREATE INDEX IF NOT EXISTS idx_tasks_repo ON agent_tasks(repo);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON agent_tasks(status);
