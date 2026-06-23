-- +goose Up
CREATE TABLE dns_publish_addresses (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  node_id uuid NOT NULL,
  address_type text NOT NULL,
  address text NOT NULL,
  source text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  observed_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (organization_id, id),
  UNIQUE (organization_id, node_id, source, address_type, address),
  CHECK (address_type IN ('A', 'AAAA')),
  CHECK (source IN ('MANUAL', 'AUTO')),
  CHECK (length(trim(address)) > 0),
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE dns_managed_records (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  dns_credential_id uuid NOT NULL,
  credential_zone_id uuid NOT NULL,
  zone_id text NOT NULL,
  zone_name text NOT NULL,
  record_host text NOT NULL DEFAULT '@',
  record_name text NOT NULL,
  record_type text NOT NULL,
  ttl integer NOT NULL DEFAULT 60,
  proxied boolean NOT NULL DEFAULT false,
  active_instance_id uuid,
  last_applied_values_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  last_evaluation_status text NOT NULL DEFAULT 'PENDING',
  last_evaluation_error text NOT NULL DEFAULT '',
  last_diagnostics_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  last_evaluated_at timestamptz,
  last_applied_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (record_type IN ('A', 'AAAA', 'CNAME')),
  CHECK (ttl > 0),
  FOREIGN KEY (organization_id, dns_credential_id) REFERENCES dns_credentials(organization_id, id),
  FOREIGN KEY (organization_id, credential_zone_id) REFERENCES dns_credential_zones(organization_id, id)
);

CREATE UNIQUE INDEX uniq_dns_managed_records_active_name
  ON dns_managed_records(organization_id, zone_id, lower(record_name), record_type)
  WHERE deleted_at IS NULL;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_dns_managed_record_type_compatibility()
RETURNS trigger AS $$
BEGIN
  IF NEW.deleted_at IS NOT NULL THEN
    RETURN NEW;
  END IF;

  PERFORM pg_advisory_xact_lock(
    hashtext(NEW.organization_id::text),
    hashtext(NEW.zone_id || ':' || lower(NEW.record_name))
  );

  IF NEW.record_type = 'CNAME' THEN
    IF EXISTS (
      SELECT 1
      FROM dns_managed_records
      WHERE organization_id = NEW.organization_id
        AND zone_id = NEW.zone_id
        AND lower(record_name) = lower(NEW.record_name)
        AND deleted_at IS NULL
        AND id <> NEW.id
    ) THEN
      RAISE EXCEPTION 'CNAME records cannot coexist with other records at the same name'
        USING ERRCODE = '23505';
    END IF;
  ELSE
    IF EXISTS (
      SELECT 1
      FROM dns_managed_records
      WHERE organization_id = NEW.organization_id
        AND zone_id = NEW.zone_id
        AND lower(record_name) = lower(NEW.record_name)
        AND record_type = 'CNAME'
        AND deleted_at IS NULL
        AND id <> NEW.id
    ) THEN
      RAISE EXCEPTION 'address records cannot coexist with a CNAME at the same name'
        USING ERRCODE = '23505';
    END IF;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER enforce_dns_managed_record_type_compatibility_on_write
  BEFORE INSERT OR UPDATE OF organization_id, zone_id, record_name, record_type, deleted_at
  ON dns_managed_records
  FOR EACH ROW EXECUTE FUNCTION enforce_dns_managed_record_type_compatibility();

CREATE TABLE dns_instances (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  managed_record_id uuid NOT NULL,
  name text NOT NULL,
  priority integer NOT NULL DEFAULT 100,
  enabled boolean NOT NULL DEFAULT true,
  node_group_ids_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  answer_count integer NOT NULL DEFAULT -1,
  condition_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  action_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  notification_channel_ids_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  last_output_values_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  last_status text NOT NULL DEFAULT 'PENDING',
  last_diagnostics_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  last_evaluated_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (priority >= 0),
  CHECK (answer_count = -1 OR answer_count > 0),
  FOREIGN KEY (organization_id, managed_record_id) REFERENCES dns_managed_records(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_dns_instances_record_priority
  ON dns_instances(managed_record_id, priority, id)
  WHERE deleted_at IS NULL;

CREATE TABLE notification_channels (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  channel_type text NOT NULL,
  config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  encrypted_secret text NOT NULL DEFAULT '',
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (channel_type IN ('WEBHOOK', 'EMAIL'))
);

CREATE TABLE notification_deliveries (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  channel_id uuid NOT NULL,
  dns_managed_record_id uuid,
  dns_instance_id uuid,
  event_type text NOT NULL,
  status text NOT NULL,
  error_message text NOT NULL DEFAULT '',
  payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL,
  delivered_at timestamptz,
  CHECK (status IN ('PENDING', 'SUCCEEDED', 'FAILED')),
  FOREIGN KEY (organization_id, channel_id) REFERENCES notification_channels(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (dns_managed_record_id) REFERENCES dns_managed_records(id) ON DELETE SET NULL,
  FOREIGN KEY (dns_instance_id) REFERENCES dns_instances(id) ON DELETE SET NULL
);

ALTER TABLE dns_managed_records
  ADD CONSTRAINT fk_dns_managed_records_active_instance
  FOREIGN KEY (active_instance_id)
  REFERENCES dns_instances(id)
  ON DELETE SET NULL;

-- +goose Down
DROP TRIGGER IF EXISTS enforce_dns_managed_record_type_compatibility_on_write ON dns_managed_records;
DROP FUNCTION IF EXISTS enforce_dns_managed_record_type_compatibility();
ALTER TABLE dns_managed_records DROP CONSTRAINT IF EXISTS fk_dns_managed_records_active_instance;
DROP TABLE IF EXISTS notification_deliveries;
DROP TABLE IF EXISTS notification_channels;
DROP TABLE IF EXISTS dns_instances;
DROP TABLE IF EXISTS dns_managed_records;
DROP TABLE IF EXISTS dns_publish_addresses;
