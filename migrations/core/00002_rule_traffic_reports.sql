-- +goose Up
CREATE TABLE rule_traffic_reports (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  agent_id uuid NOT NULL,
  report_id text NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (organization_id, agent_id, report_id),
  FOREIGN KEY (organization_id, agent_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE node_rule_traffic_assignments (
  organization_id uuid NOT NULL,
  node_id uuid NOT NULL,
  rule_id uuid NOT NULL,
  first_seen_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  PRIMARY KEY (organization_id, node_id, rule_id),
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, rule_id) REFERENCES forwarding_rules(organization_id, id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS node_rule_traffic_assignments;
DROP TABLE IF EXISTS rule_traffic_reports;
