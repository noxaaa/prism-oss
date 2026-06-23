-- +goose Up
UPDATE dns_managed_records
SET active_instance_id = NULL,
    last_evaluation_status = CASE
      WHEN last_evaluation_status = 'APPLIED' THEN 'PENDING'
      ELSE last_evaluation_status
    END,
    last_diagnostics_json = COALESCE(last_diagnostics_json, '[]'::jsonb) || jsonb_build_array(jsonb_build_object(
      'code', 'STALE_ACTIVE_INSTANCE_CLEARED',
      'message', 'The active DNS instance reference was cleared because the referenced instance is no longer enabled for this managed record.'
    )),
    updated_at = clock_timestamp()
WHERE active_instance_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM dns_instances
    WHERE dns_instances.organization_id = dns_managed_records.organization_id
      AND dns_instances.id = dns_managed_records.active_instance_id
      AND dns_instances.managed_record_id = dns_managed_records.id
      AND dns_instances.enabled = true
      AND dns_instances.deleted_at IS NULL
  );

DELETE FROM health_events
WHERE event_type IN ('DNS_FAILOVER', 'DNS_DELETE_OFFLINE', 'DNS_DELETE_ALL', 'DNS_RESTORE');

UPDATE health_evaluation_rules
SET deleted_at = COALESCE(deleted_at, clock_timestamp()),
    updated_at = clock_timestamp()
WHERE deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1
    FROM health_events
    WHERE health_events.organization_id = health_evaluation_rules.organization_id
      AND health_events.health_evaluation_rule_id = health_evaluation_rules.id
      AND health_events.deleted_at IS NULL
  );

ALTER TABLE health_events
  DROP CONSTRAINT IF EXISTS health_events_event_type_check;

ALTER TABLE health_events
  ADD CONSTRAINT health_events_event_type_check
  CHECK (event_type IN ('WEBHOOK', 'EMAIL'));

DROP TABLE IF EXISTS dns_records;

-- +goose Down
SELECT 1;
