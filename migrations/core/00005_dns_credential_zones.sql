-- +goose Up
CREATE TABLE dns_credential_zones (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  dns_credential_id uuid NOT NULL,
  zone_id text NOT NULL,
  zone_name text NOT NULL,
  status text NOT NULL,
  last_synced_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (organization_id, id),
  UNIQUE (organization_id, dns_credential_id, zone_id),
  CHECK (status IN ('ACTIVE', 'PENDING', 'MOVED', 'DELETED', 'DEACTIVATED', 'READ_ONLY', 'UNAVAILABLE', 'UNKNOWN')),
  FOREIGN KEY (organization_id, dns_credential_id) REFERENCES dns_credentials(organization_id, id) ON DELETE CASCADE
);

ALTER TABLE dns_records
  ADD COLUMN credential_zone_id uuid,
  ADD COLUMN zone_id text,
  ADD COLUMN zone_name text,
  ADD COLUMN record_host text NOT NULL DEFAULT '';

UPDATE dns_records
SET zone_id = zone,
    zone_name = zone,
    record_host = CASE
      WHEN lower(trim(trailing '.' FROM record_name)) = lower(trim(trailing '.' FROM zone)) THEN '@'
      WHEN lower(trim(trailing '.' FROM record_name)) LIKE '%.' || lower(trim(trailing '.' FROM zone))
        THEN left(
          trim(trailing '.' FROM record_name),
          length(trim(trailing '.' FROM record_name)) - length(trim(trailing '.' FROM zone)) - 1
        )
      ELSE trim(trailing '.' FROM record_name)
    END
WHERE zone_id IS NULL;

ALTER TABLE dns_records
  ALTER COLUMN zone_id SET NOT NULL,
  ALTER COLUMN zone_name SET NOT NULL;

ALTER TABLE dns_records
  ADD CONSTRAINT fk_dns_records_credential_zone
  FOREIGN KEY (organization_id, credential_zone_id) REFERENCES dns_credential_zones(organization_id, id);

CREATE INDEX idx_dns_credential_zones_credential
  ON dns_credential_zones(organization_id, dns_credential_id, zone_name);

CREATE INDEX idx_dns_records_credential_zone
  ON dns_records(organization_id, credential_zone_id)
  WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX uniq_dns_records_active_zone_id_name
  ON dns_records(organization_id, zone_id, record_name, record_type)
  WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS uniq_dns_records_active_zone_id_name;
DROP INDEX IF EXISTS idx_dns_records_credential_zone;
DROP INDEX IF EXISTS idx_dns_credential_zones_credential;

ALTER TABLE dns_records
  DROP CONSTRAINT IF EXISTS fk_dns_records_credential_zone,
  DROP COLUMN IF EXISTS credential_zone_id,
  DROP COLUMN IF EXISTS zone_id,
  DROP COLUMN IF EXISTS zone_name,
  DROP COLUMN IF EXISTS record_host;

DROP TABLE IF EXISTS dns_credential_zones;
