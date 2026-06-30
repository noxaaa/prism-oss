import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const monitorSource = () => readFileSync(join(process.cwd(), "src/components/console/features/monitors.tsx"), "utf8");
const dnsSource = () => readFileSync(join(process.cwd(), "src/components/console/features/dns.tsx"), "utf8");
const nodesSource = () => readFileSync(join(process.cwd(), "src/components/console/features/nodes.tsx"), "utf8");
const nodeMutationSource = () => readFileSync(join(process.cwd(), "src/components/console/features/node-mutation-form.tsx"), "utf8");
const nodeMetricsSource = () => readFileSync(join(process.cwd(), "src/components/console/features/node-metrics-panel.tsx"), "utf8");

describe("monitor health DNS console source", () => {
  it("keeps monitor, health, and DNS mutation forms inside drawers", () => {
    const text = monitorSource();
    const dnsText = dnsSource();

    expect(text).toContain("MonitorGroupCreateDrawer");
    expect(text).toContain("MonitorGroupEditDrawer");
    expect(text).toContain("MonitorCreateDrawer");
    expect(text).toContain("MonitorEditDrawer");
    expect(text).toContain("HealthCheckCreateDrawer");
    expect(text).toContain("HealthCheckEditDrawer");
    expect(dnsText).toContain("DNSCredentialCreateDrawer");
    expect(dnsText).toContain("DNSCredentialEditDrawer");
    expect(dnsText).toContain("DNSManagedRecordCreateDrawer");
    expect(dnsText).toContain("DNSManagedRecordEditDrawer");
    expect(dnsText).toContain("DNSInstanceCreateDrawer");
    expect(dnsText).toContain("DNSInstanceEditDrawer");
    expect(dnsText).toContain("NotificationChannelCreateDrawer");
    expect(dnsText).toContain("NotificationChannelEditDrawer");
    expect(text).not.toContain("<CardTitle>{t(\"monitors.createGroup\")}</CardTitle>");
    expect(text).not.toContain("<CardTitle>{t(\"health.create\")}</CardTitle>");
    expect(dnsText).not.toContain("<CardTitle>{t(\"dns.createCredential\")}</CardTitle>");
    expect(dnsText).not.toContain("<CardTitle>{t(\"dns.createRecord\")}</CardTitle>");
  });

  it("exposes health check result details and credential zone refresh actions", () => {
    const text = monitorSource();
    const dnsText = dnsSource();

    expect(text).toContain("/results");
    expect(text).toContain("latest_results");
    expect(dnsText).toContain("/zones/refresh");
    expect(dnsText).toContain("credential.zones");
  });

  it("uses smart DNS managed records, instances, and notification policy payloads", () => {
    const text = monitorSource();
    const dnsText = dnsSource();
    const nodesText = nodesSource();
    const nodeMutationText = nodeMutationSource();
    const nodeMetricsText = nodeMetricsSource();

    expect(text).toContain("target_scope");
    expect(text).toContain("summarizeHealthResults");
    expect(text).toContain("formatHealthLatencyMs");
    expect(text).toContain("formatHealthCheckTargets");
    expect(text).toContain("MultiSelectField");
    expect(text).toContain("form.getAll(\"target_id\")");
    expect(text).toContain("form.getAll(\"group_id\")");
    expect(text).not.toContain("existing_target_id");
    expect(text).toContain("<HealthCheckForm key={props.check?.id ?? \"empty\"}");
    expect(text).toContain("{canRead ? <Button");
    expect(text).toContain("{canUseEditor ? <Button");
    expect(text).toContain("{canManage ? <Button");
    expect(text).toContain("HealthProbeConfigFields");
    expect(text).toContain("healthProbeConfigFromForm");
    expect(text).toContain("name=\"config_port_override\"");
    expect(text).toContain("name=\"config_http_expected_statuses\"");
    expect(text).not.toContain("name=\"config\"");
    expect(text).not.toContain("JSON.stringify(check?.config");
    expect(text).toContain("function monitorGroupIDs(monitor: Monitor): string[]");
    expect(text).not.toContain("monitor.group_ids.includes");
    expect(text).not.toContain("monitor.group_ids.map");
    expect(nodesText).toContain("function nodeGroupIDs(node: NodeResource): string[]");
    expect(nodesText + nodeMutationText).toContain("send_ips");
    expect(nodesText + nodeMutationText).toContain("max_rule_ports");
    expect(nodesText + nodeMutationText).toContain("NodeSendIP");
    expect(nodesText).toContain("NodeGeoIPCell");
    expect(nodesText).toContain("NodeSystemHover");
    expect(nodesText).toContain("const metricNodes = canReadMetrics ? nodes.data : noMetricNodes");
    expect(nodesText).toContain("<NodeMetricsPanel metricsByNode={metricsByNode} nodes={metricNodes} />");
    expect(nodesText).not.toContain("MAX_NODE_METRIC_STREAMS");
    expect(nodesText).not.toContain("nodes.slice(0");
    expect(nodesText).toContain("new EventSource(\"/api/control/nodes/metrics/stream\")");
    expect(nodesText).not.toContain("const sources = nodes.map");
    expect(nodeMetricsText).toContain("NodeCPUHover");
    expect(nodeMetricsText).toContain("cpu_model");
    expect(nodeMetricsText).toContain("architecture");
    expect(nodeMetricsText).toContain("virtualization_system");
    expect(nodeMetricsText).toContain("disk_used_bytes");
    expect(nodeMetricsText).toContain("disk_total_bytes");
    expect(nodeMetricsText).toContain("diskPercent");
    expect(nodeMetricsText).toContain("hasDiskMetrics");
    expect(nodeMetricsText).toContain("nodes.metricsWaiting");
    expect(nodeMetricsText).toContain("const metrics = metricsByNode[node.id];");
    expect(nodeMetricsText).not.toContain("metricsByNode[node.id] ?? {}");
    expect(nodesText).toContain("geoip");
    expect(nodesText).toContain("https://db-ip.com/db/download/ip-to-country-lite");
    expect(nodeMetricsText).toContain("positiveCountOrUnknown(metrics.cpu_logical_cores");
    expect(nodesText).not.toContain("node.group_ids.includes");
    expect(nodesText).not.toContain("node.group_ids.map");
    expect(dnsText).toContain("/api/control/dns/managed-records");
    expect(dnsText).toContain("/api/control/dns/instances");
    expect(dnsText).toContain("/api/control/notification-channels");
    expect(dnsText).toContain("<NotificationChannelForm key={channel?.id ?? \"empty\"}");
    expect(dnsText).toContain("node_group_ids");
    expect(dnsText).toContain("name=\"node_group_id\" options={nodeGroups");
    expect(dnsText).toContain("name=\"notification_channel_id\" options={channels");
    expect(dnsText).toContain("required={false}");
    expect(dnsText).toContain("answer_count");
    expect(dnsText).toContain("DNSConditionBuilder");
    expect(dnsText).toContain("data-testid=\"dns-condition-builder\"");
    expect(dnsText).toContain("sm:grid-cols-[minmax(0,1fr)_4.5rem_5rem_2rem]");
    expect(dnsText).toContain("h-8 w-full min-w-0 rounded-md border bg-background px-2.5 text-sm");
    expect(dnsText).toContain("data-testid=\"dns-condition-add-condition\"");
    expect(dnsText).toContain("name=\"condition_payload\"");
    expect(dnsText).not.toContain("name=\"condition_json\"");
    expect(dnsText).toContain("ROTATE_ONLINE_NODES");
    expect(dnsText).toContain("SET_STATIC_ADDRESSES");
    expect(dnsText).toContain("SET_STATIC_CNAME");
    expect(dnsText).toContain("USE_INSTANCE_OUTPUT");
    expect(dnsText).not.toContain("failover" + "_values");
    expect(dnsText).not.toContain("preserve_health_binding");
    expect(dnsText).not.toContain("dns-" + "record-form");
    expect(existsSync(join(process.cwd(), "src/components/console/dns-" + "record-form.ts"))).toBe(false);
  });

  it("preserves non-destructive edit state for DNS zones and harmless node publish address edits", () => {
    const dnsText = dnsSource();
    const nodesText = nodesSource();
    const nodeMutationText = nodeMutationSource();

    expect(dnsText).toContain("zone.id === currentZoneID");
    expect(dnsText).toContain("candidate.id === nextCurrentZoneID");
    expect(nodeMutationText).toContain("publishAddressChanged");
    expect(nodeMutationText).toContain("payload.dns_publish_addresses = nodeDNSPublishAddressPayload(node, publishAddress)");
    expect(nodeMutationText).toContain("address.source === \"MANUAL\" && address.address === primaryAddress");
    expect(nodeMutationText).not.toContain("...rest");
    expect(nodeMutationText).not.toContain("rest.map(nodeDNSPublishAddressPayloadItem)");
  });
});
