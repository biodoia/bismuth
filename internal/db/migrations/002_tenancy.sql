-- 002_tenancy.sql — P7-e namespace isolation (V1).
--
-- Single shared SQLite DB with a tenant column on the multi-tenant
-- surfaces (agents, tasks). Per-team databases remain a V2 upgrade
-- path; this gives API-level namespace scoping today.

ALTER TABLE agents ADD COLUMN tenant TEXT NOT NULL DEFAULT 'default';
ALTER TABLE tasks  ADD COLUMN tenant TEXT NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents(tenant);
CREATE INDEX IF NOT EXISTS idx_tasks_tenant  ON tasks(tenant);
