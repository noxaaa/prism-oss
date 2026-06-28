-- +goose Up
CREATE TABLE node_send_ips (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL,
  node_id uuid NOT NULL,
  send_ip text NOT NULL,
  display_name text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (organization_id, node_id, send_ip),
  CHECK (length(trim(send_ip)) > 0),
  CHECK (length(trim(display_name)) > 0),
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE
);

ALTER TABLE nodes
  ADD COLUMN max_rule_ports integer NOT NULL DEFAULT 256,
  ADD CONSTRAINT nodes_max_rule_ports_check CHECK (max_rule_ports BETWEEN 1 AND 65535);

ALTER TABLE node_enrollment_profiles
  ADD COLUMN send_ips_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN max_rule_ports integer NOT NULL DEFAULT 256,
  ADD CONSTRAINT node_enrollment_profiles_max_rule_ports_check CHECK (max_rule_ports BETWEEN 1 AND 65535);

CREATE TABLE inbound_binding_port_segments (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL,
  inbound_binding_id uuid NOT NULL,
  start_port integer NOT NULL,
  end_port integer NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (organization_id, inbound_binding_id, start_port, end_port),
  CHECK (start_port BETWEEN 1 AND 65535),
  CHECK (end_port BETWEEN 1 AND 65535),
  CHECK (start_port <= end_port),
  FOREIGN KEY (organization_id, inbound_binding_id) REFERENCES inbound_bindings(organization_id, id) ON DELETE CASCADE
);

INSERT INTO inbound_binding_port_segments (id, organization_id, inbound_binding_id, start_port, end_port, created_at)
SELECT gen_random_uuid(), organization_id, id, port, port, created_at
FROM inbound_bindings;

INSERT INTO node_port_ranges (id, organization_id, node_id, protocol, start_port, end_port, enabled, created_at, updated_at)
SELECT gen_random_uuid(), nodes.organization_id, nodes.id, defaults.protocol, 1, 65535, true, nodes.created_at, nodes.updated_at
FROM nodes
CROSS JOIN (VALUES ('TCP'), ('UDP')) AS defaults(protocol)
WHERE NOT EXISTS (
  SELECT 1
  FROM node_port_ranges
  WHERE node_port_ranges.organization_id = nodes.organization_id
    AND node_port_ranges.node_id = nodes.id
);

-- +goose StatementBegin
DO $$
DECLARE
  endpoint_constraint_name text;
BEGIN
  SELECT pg_constraint.conname
    INTO endpoint_constraint_name
  FROM pg_constraint
  WHERE pg_constraint.conrelid = 'inbound_bindings'::regclass
    AND pg_constraint.contype = 'u'
    AND (
      SELECT array_agg(pg_attribute.attname::text ORDER BY keys.ordinality)
      FROM unnest(pg_constraint.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
      JOIN pg_attribute
        ON pg_attribute.attrelid = pg_constraint.conrelid
       AND pg_attribute.attnum = keys.attnum
    ) = ARRAY['organization_id', 'node_group_id', 'listen_ip', 'protocol', 'port', 'match_type'];

  IF endpoint_constraint_name IS NOT NULL THEN
    EXECUTE format('ALTER TABLE inbound_bindings DROP CONSTRAINT %I', endpoint_constraint_name);
  END IF;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_default_inbound_binding_port_segment()
RETURNS trigger AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM inbound_binding_port_segments
    WHERE organization_id = NEW.organization_id
      AND inbound_binding_id = NEW.id
  ) THEN
    INSERT INTO inbound_binding_port_segments (
      id, organization_id, inbound_binding_id, start_port, end_port, created_at
    )
    VALUES (
      gen_random_uuid(), NEW.organization_id, NEW.id, NEW.port, NEW.port, NEW.created_at
    )
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER ensure_inbound_binding_port_segment_insert
  AFTER INSERT ON inbound_bindings
  FOR EACH ROW EXECUTE FUNCTION ensure_default_inbound_binding_port_segment();

ALTER TABLE forwarding_rules
  ADD COLUMN send_ip text NOT NULL DEFAULT '';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_forwarding_rule()
RETURNS trigger AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM inbound_bindings
    WHERE inbound_bindings.organization_id = NEW.organization_id
      AND inbound_bindings.id = NEW.inbound_binding_id
      AND inbound_bindings.protocol = NEW.protocol
      AND inbound_bindings.match_type = NEW.match_type
  ) THEN
    RAISE EXCEPTION 'forwarding rule inbound binding must match protocol and match type';
  END IF;

  IF NEW.deleted_at IS NULL AND NEW.enabled AND NEW.status = 'ENABLED' THEN
    IF NEW.match_type = 'ANY_INBOUND' AND EXISTS (
      SELECT 1
      FROM forwarding_rules AS existing_rule
      JOIN inbound_bindings AS existing_binding
        ON existing_binding.organization_id = existing_rule.organization_id
       AND existing_binding.id = existing_rule.inbound_binding_id
      JOIN inbound_bindings AS new_binding
        ON new_binding.organization_id = NEW.organization_id
       AND new_binding.id = NEW.inbound_binding_id
      JOIN inbound_binding_port_segments AS existing_segment
        ON existing_segment.organization_id = existing_binding.organization_id
       AND existing_segment.inbound_binding_id = existing_binding.id
      JOIN inbound_binding_port_segments AS new_segment
        ON new_segment.organization_id = new_binding.organization_id
       AND new_segment.inbound_binding_id = new_binding.id
      WHERE existing_rule.id != NEW.id
        AND existing_rule.organization_id = NEW.organization_id
        AND existing_rule.enabled
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR btrim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR btrim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_segment.start_port <= new_segment.end_port
        AND new_segment.start_port <= existing_segment.end_port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
    ) THEN
      RAISE EXCEPTION 'ANY_INBOUND rule already exists on this inbound endpoint';
    END IF;

    IF NEW.match_type = 'TLS_SNI' AND EXISTS (
      SELECT 1
      FROM forwarding_rules AS existing_rule
      JOIN inbound_bindings AS existing_binding
        ON existing_binding.organization_id = existing_rule.organization_id
       AND existing_binding.id = existing_rule.inbound_binding_id
      JOIN inbound_bindings AS new_binding
        ON new_binding.organization_id = NEW.organization_id
       AND new_binding.id = NEW.inbound_binding_id
      JOIN inbound_binding_port_segments AS existing_segment
        ON existing_segment.organization_id = existing_binding.organization_id
       AND existing_segment.inbound_binding_id = existing_binding.id
      JOIN inbound_binding_port_segments AS new_segment
        ON new_segment.organization_id = new_binding.organization_id
       AND new_segment.inbound_binding_id = new_binding.id
      WHERE existing_rule.id != NEW.id
        AND existing_rule.organization_id = NEW.organization_id
        AND existing_rule.enabled
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR btrim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR btrim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_segment.start_port <= new_segment.end_port
        AND new_segment.start_port <= existing_segment.end_port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
        AND (
          existing_rule.match_type != 'TLS_SNI'
          OR btrim(existing_binding.listen_ip, '[]') != btrim(new_binding.listen_ip, '[]')
          OR lower(existing_rule.sni_hostname) = lower(NEW.sni_hostname)
        )
    ) THEN
      RAISE EXCEPTION 'TLS_SNI rule already exists on this inbound endpoint';
    END IF;

    IF NEW.match_type NOT IN ('ANY_INBOUND', 'TLS_SNI') AND EXISTS (
      SELECT 1
      FROM forwarding_rules AS existing_rule
      JOIN inbound_bindings AS existing_binding
        ON existing_binding.organization_id = existing_rule.organization_id
       AND existing_binding.id = existing_rule.inbound_binding_id
      JOIN inbound_bindings AS new_binding
        ON new_binding.organization_id = NEW.organization_id
       AND new_binding.id = NEW.inbound_binding_id
      JOIN inbound_binding_port_segments AS existing_segment
        ON existing_segment.organization_id = existing_binding.organization_id
       AND existing_segment.inbound_binding_id = existing_binding.id
      JOIN inbound_binding_port_segments AS new_segment
        ON new_segment.organization_id = new_binding.organization_id
       AND new_segment.inbound_binding_id = new_binding.id
      WHERE existing_rule.id != NEW.id
        AND existing_rule.organization_id = NEW.organization_id
        AND existing_rule.enabled
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR btrim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR btrim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_segment.start_port <= new_segment.end_port
        AND new_segment.start_port <= existing_segment.end_port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
    ) THEN
      RAISE EXCEPTION 'unsupported match type conflicts with existing inbound endpoint';
    END IF;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE forwarding_rules
  DROP COLUMN IF EXISTS send_ip;

DROP TRIGGER IF EXISTS ensure_inbound_binding_port_segment_insert ON inbound_bindings;
DROP FUNCTION IF EXISTS ensure_default_inbound_binding_port_segment();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_forwarding_rule()
RETURNS trigger AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM inbound_bindings
    WHERE inbound_bindings.organization_id = NEW.organization_id
      AND inbound_bindings.id = NEW.inbound_binding_id
      AND inbound_bindings.protocol = NEW.protocol
      AND inbound_bindings.match_type = NEW.match_type
  ) THEN
    RAISE EXCEPTION 'forwarding rule inbound binding must match protocol and match type';
  END IF;

  IF NEW.deleted_at IS NULL AND NEW.enabled AND NEW.status = 'ENABLED' THEN
    IF NEW.match_type = 'ANY_INBOUND' AND EXISTS (
      SELECT 1
      FROM forwarding_rules AS existing_rule
      JOIN inbound_bindings AS existing_binding
        ON existing_binding.organization_id = existing_rule.organization_id
       AND existing_binding.id = existing_rule.inbound_binding_id
      JOIN inbound_bindings AS new_binding
        ON new_binding.organization_id = NEW.organization_id
       AND new_binding.id = NEW.inbound_binding_id
      WHERE existing_rule.id != NEW.id
        AND existing_rule.organization_id = NEW.organization_id
        AND existing_rule.enabled
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR btrim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR btrim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
    ) THEN
      RAISE EXCEPTION 'ANY_INBOUND rule already exists on this inbound endpoint';
    END IF;

    IF NEW.match_type = 'TLS_SNI' AND EXISTS (
      SELECT 1
      FROM forwarding_rules AS existing_rule
      JOIN inbound_bindings AS existing_binding
        ON existing_binding.organization_id = existing_rule.organization_id
       AND existing_binding.id = existing_rule.inbound_binding_id
      JOIN inbound_bindings AS new_binding
        ON new_binding.organization_id = NEW.organization_id
       AND new_binding.id = NEW.inbound_binding_id
      WHERE existing_rule.id != NEW.id
        AND existing_rule.organization_id = NEW.organization_id
        AND existing_rule.enabled
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR btrim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR btrim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
        AND (
          existing_rule.match_type != 'TLS_SNI'
          OR btrim(existing_binding.listen_ip, '[]') != btrim(new_binding.listen_ip, '[]')
          OR lower(existing_rule.sni_hostname) = lower(NEW.sni_hostname)
        )
    ) THEN
      RAISE EXCEPTION 'TLS_SNI rule already exists on this inbound endpoint';
    END IF;

    IF NEW.match_type NOT IN ('ANY_INBOUND', 'TLS_SNI') AND EXISTS (
      SELECT 1
      FROM forwarding_rules AS existing_rule
      JOIN inbound_bindings AS existing_binding
        ON existing_binding.organization_id = existing_rule.organization_id
       AND existing_binding.id = existing_rule.inbound_binding_id
      JOIN inbound_bindings AS new_binding
        ON new_binding.organization_id = NEW.organization_id
       AND new_binding.id = NEW.inbound_binding_id
      WHERE existing_rule.id != NEW.id
        AND existing_rule.organization_id = NEW.organization_id
        AND existing_rule.enabled
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR btrim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR btrim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
    ) THEN
      RAISE EXCEPTION 'unsupported match type conflicts with existing inbound endpoint';
    END IF;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TABLE IF EXISTS inbound_binding_port_segments;

ALTER TABLE nodes
  DROP CONSTRAINT IF EXISTS nodes_max_rule_ports_check,
  DROP COLUMN IF EXISTS max_rule_ports;

ALTER TABLE node_enrollment_profiles
  DROP CONSTRAINT IF EXISTS node_enrollment_profiles_max_rule_ports_check,
  DROP COLUMN IF EXISTS max_rule_ports,
  DROP COLUMN IF EXISTS send_ips_json;

DROP TABLE IF EXISTS node_send_ips;

ALTER TABLE inbound_bindings
  ADD CONSTRAINT inbound_bindings_endpoint_unique UNIQUE (organization_id, node_group_id, listen_ip, protocol, port, match_type);
