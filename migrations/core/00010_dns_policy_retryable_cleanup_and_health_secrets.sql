-- +goose Up
ALTER TABLE dns_managed_records
  ADD COLUMN IF NOT EXISTS provider_retirements_json jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE health_events
  ADD COLUMN IF NOT EXISTS encrypted_secret text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE health_events
  DROP COLUMN IF EXISTS encrypted_secret;
