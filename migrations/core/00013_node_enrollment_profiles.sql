-- +goose Up
SET search_path TO app, auth, public;

ALTER TABLE nodes
  ADD COLUMN enrollment_profile_id uuid;

CREATE TABLE node_enrollment_profiles (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  token_hash text NOT NULL UNIQUE,
  enabled boolean NOT NULL DEFAULT true,
  expires_at timestamptz,
  max_uses integer NOT NULL DEFAULT 0,
  used_count integer NOT NULL DEFAULT 0,
  node_name_template text NOT NULL DEFAULT '{{hostname}}',
  group_ids_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  listen_ips_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  port_ranges_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  dns_publish_addresses_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  dataplane_mode text NOT NULL DEFAULT 'AUTO',
  dataplane_conflict_policy text NOT NULL DEFAULT 'FAIL_FAST',
  auto_update_enabled boolean NOT NULL DEFAULT true,
  allowed_cidrs_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_by_user_id text REFERENCES "user"(id),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  revoked_at timestamptz,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (max_uses >= 0),
  CHECK (used_count >= 0),
  CHECK (dataplane_mode IN ('AUTO', 'NATIVE', 'HAPROXY', 'NFTABLES')),
  CHECK (dataplane_conflict_policy IN ('FAIL_FAST'))
);

CREATE INDEX node_enrollment_profiles_org_idx
  ON node_enrollment_profiles(organization_id)
  WHERE deleted_at IS NULL;

ALTER TABLE nodes
  ADD CONSTRAINT nodes_enrollment_profile_fk
  FOREIGN KEY (organization_id, enrollment_profile_id)
  REFERENCES node_enrollment_profiles(organization_id, id)
  ON DELETE SET NULL (enrollment_profile_id);

CREATE TABLE node_enrollment_events (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  enrollment_profile_id uuid NOT NULL,
  node_id uuid,
  status text NOT NULL,
  reason_code text NOT NULL DEFAULT '',
  message text NOT NULL DEFAULT '',
  remote_ip text NOT NULL DEFAULT '',
  hostname text NOT NULL DEFAULT '',
  metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL,
  CHECK (status IN ('SUCCEEDED', 'FAILED')),
  FOREIGN KEY (organization_id, enrollment_profile_id)
    REFERENCES node_enrollment_profiles(organization_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (organization_id, node_id)
    REFERENCES nodes(organization_id, id)
    ON DELETE SET NULL (node_id)
);

CREATE INDEX node_enrollment_events_profile_idx
  ON node_enrollment_events(organization_id, enrollment_profile_id, created_at DESC);

ALTER TABLE agent_credentials
  ADD COLUMN enrollment_profile_id uuid;

ALTER TABLE agent_credentials
  ADD COLUMN enrollment_token_hash text NOT NULL DEFAULT '';

ALTER TABLE agent_credentials
  ADD CONSTRAINT agent_credentials_enrollment_profile_fk
  FOREIGN KEY (organization_id, enrollment_profile_id)
  REFERENCES node_enrollment_profiles(organization_id, id)
  ON DELETE SET NULL (enrollment_profile_id);

CREATE INDEX agent_credentials_pending_enrollment_profile_idx
  ON agent_credentials(organization_id, enrollment_profile_id, created_at, id)
  WHERE enrollment_profile_id IS NOT NULL
    AND activated_at IS NULL
    AND revoked_at IS NULL;

-- +goose Down
SET search_path TO app, auth, public;

DROP INDEX IF EXISTS agent_credentials_pending_enrollment_profile_idx;

ALTER TABLE agent_credentials DROP CONSTRAINT IF EXISTS agent_credentials_enrollment_profile_fk;
ALTER TABLE agent_credentials DROP COLUMN IF EXISTS enrollment_token_hash;
ALTER TABLE agent_credentials DROP COLUMN IF EXISTS enrollment_profile_id;

DROP TABLE IF EXISTS node_enrollment_events;

ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_enrollment_profile_fk;
ALTER TABLE nodes DROP COLUMN IF EXISTS enrollment_profile_id;

DROP TABLE IF EXISTS node_enrollment_profiles;
