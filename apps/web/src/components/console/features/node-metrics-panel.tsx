"use client";

import { HoverCard, HoverCardContent, HoverCardTrigger } from "@/components/ui/hover-card";
import { Progress } from "@/components/ui/progress";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { bytes, formatBitrateBps, shortDate } from "@/components/console/control-api";
import { useI18n } from "@/components/console/i18n";
import { StatusBadge, duration, percent } from "@/components/console/shared";
import type { AgentMetrics, NodeResource } from "@/components/console/types";

export function NodeMetricsPanel({ metricsByNode, nodes }: { metricsByNode: Record<string, AgentMetrics>; nodes: NodeResource[] }) {
  const { locale, t } = useI18n();

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("nodes.nodeMetrics")}</CardTitle>
        <CardDescription>{t("nodes.nodeMetricsDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="min-w-0 overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("overview.node")}</TableHead>
                <TableHead>{t("overview.status")}</TableHead>
                <TableHead>{t("nodes.bandwidth")}</TableHead>
                <TableHead>CPU</TableHead>
                <TableHead>RAM</TableHead>
                <TableHead>{t("nodes.disk")}</TableHead>
                <TableHead>{t("nodes.uptime")}</TableHead>
                <TableHead>{t("nodes.config")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {nodes.map((node) => {
                const metrics = metricsByNode[node.id];
                return (
                  <TableRow key={node.id}>
                    <TableCell>
                      <div className="flex flex-col gap-1">
                        <NodeNameHover metrics={metrics} node={node} />
                        {node.public_description ? <span className="text-xs text-muted-foreground">{node.public_description}</span> : null}
                      </div>
                    </TableCell>
                    <TableCell><StatusBadge value={metrics?.status ?? node.status} /></TableCell>
                    <TableCell>
                      <HoverCard>
                        <HoverCardTrigger asChild>
                          <span className="inline-flex cursor-default font-medium">{metrics ? formatBitrateBps(metrics.bandwidth_bps) : t("nodes.metricsNotStreamed")}</span>
                        </HoverCardTrigger>
                        <HoverCardContent align="start">
                          {metrics ? (
                            <>
                              <MetricDetail label={t("usage.upload")} value={bytes(metrics.upload_bytes)} />
                              <MetricDetail label={t("usage.download")} value={bytes(metrics.download_bytes)} />
                              <MetricDetail label="TCP" value={metrics.tcp_connections ?? 0} />
                              <MetricDetail label="UDP/s" value={metrics.udp_packets_per_second ?? 0} />
                            </>
                          ) : (
                            <MetricDetail label={t("nodes.metrics")} value={t("nodes.metricsNotStreamed")} />
                          )}
                        </HoverCardContent>
                      </HoverCard>
                    </TableCell>
                    <TableCell><NodeCPUHover metrics={metrics} /></TableCell>
                    <TableCell>
                      {metrics ? (
                        <HoverCard>
                          <HoverCardTrigger asChild>
                            <div className="inline-flex w-32 cursor-default"><MetricProgress value={ramPercent(metrics)} label={ramLabel(metrics, t("nodes.metricsNotStreamed"))} /></div>
                          </HoverCardTrigger>
                          <HoverCardContent align="start">
                            <MetricDetail label="RAM" value={ramDetail(metrics, t("nodes.metricsNotStreamed"))} />
                          </HoverCardContent>
                        </HoverCard>
                      ) : (
                        <span className="text-sm text-muted-foreground">{t("nodes.metricsNotStreamed")}</span>
                      )}
                    </TableCell>
                    <TableCell>
                      {metrics ? (
                        <HoverCard>
                          <HoverCardTrigger asChild>
                            <div className="inline-flex w-32 cursor-default"><MetricProgress value={diskPercent(metrics)} label={diskLabel(metrics, t("nodes.metricsNotStreamed"))} /></div>
                          </HoverCardTrigger>
                          <HoverCardContent align="start">
                            <MetricDetail label={t("nodes.disk")} value={diskDetail(metrics, t("nodes.metricsNotStreamed"))} />
                          </HoverCardContent>
                        </HoverCard>
                      ) : (
                        <span className="text-sm text-muted-foreground">{t("nodes.metricsNotStreamed")}</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <HoverCard>
                        <HoverCardTrigger asChild>
                          <span className="inline-flex cursor-default">{metrics ? duration(metrics.uptime_seconds) : t("nodes.metricsNotStreamed")}</span>
                        </HoverCardTrigger>
                        <HoverCardContent align="start">
                          <MetricDetail label={t("nodes.bootTime")} value={metrics ? shortDate(metrics.boot_time, locale) : t("nodes.metricsNotStreamed")} />
                          <MetricDetail label={t("overview.lastSeen")} value={metrics ? shortDate(metrics.last_seen_at ?? node.last_seen_at, locale) : shortDate(node.last_seen_at, locale)} />
                        </HoverCardContent>
                      </HoverCard>
                    </TableCell>
                    <TableCell>{metrics?.applied_config_version ?? node.applied_config_version}/{metrics?.desired_config_version ?? node.desired_config_version}</TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  );
}

function NodeNameHover({ metrics, node }: { metrics: AgentMetrics | undefined; node: NodeResource }) {
  const { t } = useI18n();
  return (
    <HoverCard>
      <HoverCardTrigger asChild>
        <span className="inline-flex max-w-56 cursor-default truncate font-medium">{node.name}</span>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-80">
        <MetricDetail label={t("nodes.osName")} value={metrics?.os_name || t("common.unknown")} />
        <MetricDetail label={t("nodes.osVersion")} value={metrics?.os_version || t("common.unknown")} />
        <MetricDetail label={t("nodes.kernelVersion")} value={metrics?.kernel_version || t("common.unknown")} />
        <MetricDetail label={t("nodes.architecture")} value={metrics?.architecture || t("common.unknown")} />
        <MetricDetail label={t("nodes.virtualization")} value={virtualizationLabel(metrics, t("common.unknown"))} />
      </HoverCardContent>
    </HoverCard>
  );
}

function MetricProgress({ label, value }: { label: string; value: number | undefined }) {
  const normalized = Math.max(0, Math.min(100, value ?? 0));
  return (
    <div className="flex min-w-28 items-center gap-2">
      <Progress className="w-16" value={normalized} />
      <span className="tabular-nums text-xs text-muted-foreground">{label}</span>
    </div>
  );
}

function NodeCPUHover({ metrics }: { metrics: AgentMetrics | undefined }) {
  const { t } = useI18n();
  if (!metrics) {
    return <span className="text-sm text-muted-foreground">{t("nodes.metricsNotStreamed")}</span>;
  }
  return (
    <HoverCard>
      <HoverCardTrigger asChild>
        <div className="inline-flex cursor-default">
          <MetricProgress value={metrics.cpu_percent} label={percent(metrics.cpu_percent)} />
        </div>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-80">
        <MetricDetail label={t("nodes.cpuModel")} value={metrics.cpu_model || t("common.unknown")} />
        <MetricDetail label={t("nodes.cpuLogicalCores")} value={positiveCountOrUnknown(metrics.cpu_logical_cores, t("common.unknown"))} />
        <MetricDetail label={t("nodes.cpuPhysicalCores")} value={positiveCountOrUnknown(metrics.cpu_physical_cores, t("common.unknown"))} />
      </HoverCardContent>
    </HoverCard>
  );
}

function MetricDetail({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 py-1">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 break-words text-right font-medium tabular-nums">{value}</span>
    </div>
  );
}

function virtualizationLabel(metrics: AgentMetrics | undefined, fallback: string) {
  const system = metrics?.virtualization_system?.trim();
  const role = metrics?.virtualization_role?.trim();
  if (system && role) {
    return `${system} (${role})`;
  }
  return system || role || fallback;
}

function positiveCountOrUnknown(value: number | undefined, fallback: string) {
  return typeof value === "number" && value > 0 ? value : fallback;
}

function ramPercent(metrics: AgentMetrics | undefined): number | undefined {
  if (!metrics?.ram_total_bytes) {
    return undefined;
  }
  return (Math.max(0, metrics.ram_used_bytes ?? 0) / metrics.ram_total_bytes) * 100;
}

function ramLabel(metrics: AgentMetrics | undefined, fallback: string): string {
  if (!metrics) {
    return fallback;
  }
  if (!metrics.ram_total_bytes) {
    return bytes(metrics.ram_used_bytes);
  }
  return percent(ramPercent(metrics));
}

function ramDetail(metrics: AgentMetrics | undefined, fallback: string): string {
  if (!metrics) {
    return fallback;
  }
  const used = bytes(metrics.ram_used_bytes);
  if (!metrics.ram_total_bytes) {
    return used;
  }
  return `${used} / ${bytes(metrics.ram_total_bytes)}`;
}

function diskPercent(metrics: AgentMetrics | undefined): number | undefined {
  const total = metrics?.disk_total_bytes ?? 0;
  if (!hasDiskMetrics(metrics) || total <= 0) {
    return undefined;
  }
  return (Math.max(0, metrics.disk_used_bytes ?? 0) / total) * 100;
}

function diskLabel(metrics: AgentMetrics | undefined, fallback: string): string {
  if (!hasDiskMetrics(metrics)) {
    return fallback;
  }
  return percent(diskPercent(metrics));
}

function diskDetail(metrics: AgentMetrics | undefined, fallback: string): string {
  if (!hasDiskMetrics(metrics)) {
    return fallback;
  }
  const used = bytes(metrics.disk_used_bytes);
  return `${used} / ${bytes(metrics.disk_total_bytes)}`;
}

function hasDiskMetrics(metrics: AgentMetrics | undefined): metrics is AgentMetrics {
  return Boolean(metrics && ((metrics.disk_total_bytes ?? 0) > 0 || (metrics.disk_used_bytes ?? 0) > 0));
}
