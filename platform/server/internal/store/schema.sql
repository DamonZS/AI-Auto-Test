CREATE TABLE IF NOT EXISTS tenants (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  plan TEXT NOT NULL DEFAULT 'standard',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS tenant_settings (
  tenant_id TEXT PRIMARY KEY,
  provider TEXT NOT NULL DEFAULT 'openai-compatible',
  model TEXT NOT NULL DEFAULT '',
  base_url TEXT NOT NULL DEFAULT '',
  api_key_ciphertext TEXT NOT NULL DEFAULT '',
  max_steps INTEGER NOT NULL DEFAULT 40,
  max_tokens INTEGER NOT NULL DEFAULT 20000,
  max_duration_seconds INTEGER NOT NULL DEFAULT 600,
  ego_enabled INTEGER NOT NULL DEFAULT 0,
  laya_enabled INTEGER NOT NULL DEFAULT 0,
  jev_enabled INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  name TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'tester',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  allowed_hosts TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS requirements (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  code TEXT NOT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS versions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  name TEXT NOT NULL,
  commit_ref TEXT NOT NULL DEFAULT '',
  impact_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS environments (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'platform',
  status TEXT NOT NULL DEFAULT 'unknown',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS cases (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  environment_id TEXT NOT NULL DEFAULT '',
  requirement_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  type TEXT NOT NULL DEFAULT 'ui',
  scenario TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft',
  expected_source TEXT NOT NULL DEFAULT 'requirement',
  generation_mode TEXT NOT NULL DEFAULT 'manual',
  dataset_id TEXT NOT NULL DEFAULT '',
  data_row_id TEXT NOT NULL DEFAULT '',
  version_no INTEGER NOT NULL DEFAULT 1,
  reviewer TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS case_revisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tenant_id TEXT NOT NULL,
  case_id TEXT NOT NULL,
  version_no INTEGER NOT NULL,
  scenario TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (tenant_id, case_id, version_no)
);

CREATE INDEX IF NOT EXISTS idx_case_revisions_history
  ON case_revisions(tenant_id, case_id, version_no);

CREATE TABLE IF NOT EXISTS test_data_sets (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  environment_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  fields_json TEXT NOT NULL DEFAULT '[]',
  record_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS test_data_records (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,
  row_no INTEGER NOT NULL,
  values_json TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_test_data_records_dataset
  ON test_data_records(tenant_id, dataset_id, row_no);

CREATE TABLE IF NOT EXISTS suites (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  name TEXT NOT NULL,
  case_ids TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS suite_runs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  suite_id TEXT NOT NULL REFERENCES suites(id)
);

CREATE INDEX IF NOT EXISTS idx_suite_runs_suite ON suite_runs(tenant_id, suite_id);

CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  case_id TEXT NOT NULL,
  version_id TEXT NOT NULL DEFAULT '',
  suite_run_id TEXT NOT NULL DEFAULT '',
  dataset_id TEXT NOT NULL DEFAULT '',
  data_row_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'queued',
  passed INTEGER,
  needs_review INTEGER NOT NULL DEFAULT 0,
  self_healed INTEGER NOT NULL DEFAULT 0,
  token_in INTEGER NOT NULL DEFAULT 0,
  token_out INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  report TEXT NOT NULL DEFAULT '',
  fallback_reason TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT (datetime('now')),
  ended_at TEXT
);

CREATE TABLE IF NOT EXISTS case_schedules (
  case_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  weekday INTEGER NOT NULL,
  hour INTEGER NOT NULL,
  minute INTEGER NOT NULL,
  last_run_key TEXT NOT NULL DEFAULT '',
  last_run_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS suite_schedules (
  suite_id TEXT NOT NULL REFERENCES suites(id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL,
  enabled INTEGER NOT NULL,
  weekday INTEGER NOT NULL,
  hour INTEGER NOT NULL,
  minute INTEGER NOT NULL,
  PRIMARY KEY (suite_id, tenant_id)
);

CREATE TABLE IF NOT EXISTS bugs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  bug_code TEXT NOT NULL,
  title TEXT NOT NULL,
  severity TEXT NOT NULL DEFAULT 'S3',
  priority TEXT NOT NULL DEFAULT 'P2',
  status TEXT NOT NULL DEFAULT 'new',
  signature TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  evidence TEXT NOT NULL DEFAULT '',
  reopen_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bugs_code ON bugs(tenant_id, bug_code);
CREATE INDEX IF NOT EXISTS idx_bugs_sig ON bugs(tenant_id, signature, status);

CREATE TABLE IF NOT EXISTS bug_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tenant_id TEXT NOT NULL,
  bug_id TEXT NOT NULL,
  from_status TEXT NOT NULL,
  to_status TEXT NOT NULL,
  actor TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS heal_patches (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  step_seq INTEGER NOT NULL DEFAULT 0,
  old_selector TEXT NOT NULL DEFAULT '',
  new_selector TEXT NOT NULL DEFAULT '',
  verdict TEXT NOT NULL DEFAULT 'drift',
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tenant_id TEXT NOT NULL,
  actor TEXT NOT NULL DEFAULT '',
  channel TEXT NOT NULL DEFAULT 'web',
  action TEXT NOT NULL,
  target TEXT NOT NULL DEFAULT '',
  at TEXT NOT NULL DEFAULT (datetime('now'))
);
