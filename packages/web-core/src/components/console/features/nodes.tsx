"use client";

import {
  ActivityIcon,
  CopyIcon,
  DownloadIcon,
  MoreHorizontalIcon,
  NetworkIcon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  ServerIcon,
  Trash2Icon,
} from "lucide-react";
import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@noxaaa/prism-oss-web-core/ui/alert";
import { Button } from "@noxaaa/prism-oss-web-core/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@noxaaa/prism-oss-web-core/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@noxaaa/prism-oss-web-core/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@noxaaa/prism-oss-web-core/ui/dropdown-menu";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@noxaaa/prism-oss-web-core/ui/field";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@noxaaa/prism-oss-web-core/ui/hover-card";
import { Separator } from "@noxaaa/prism-oss-web-core/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@noxaaa/prism-oss-web-core/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@noxaaa/prism-oss-web-core/ui/table";
import { Textarea } from "@noxaaa/prism-oss-web-core/ui/textarea";
import { controlDelete, controlPatch, controlPost, shortDate } from "@noxaaa/prism-oss-web-core/console/control-api";
import { localizeControlError, localizeEnum, useI18n } from "@noxaaa/prism-oss-web-core/console/i18n";
import { hasPermission } from "@noxaaa/prism-oss-web-core/console/permissions";
import { useConsoleSession } from "@noxaaa/prism-oss-web-core/console/shell";
import {
  NodeEnrollmentCreateDrawer,
  NodeEnrollmentDeleteDialog,
  NodeEnrollmentDetailDrawer,
  NodeEnrollmentEditDrawer,
} from "@noxaaa/prism-oss-web-core/console/features/node-enrollment-profiles";
import { NodeMutationForm } from "@noxaaa/prism-oss-web-core/console/features/node-mutation-form";
import { NodeMetricsPanel } from "@noxaaa/prism-oss-web-core/console/features/node-metrics-panel";
import {
  DataState,
  PageStack,
  StatusBadge,
  SummaryCard,
  SummaryGrid,
  TableSkeleton,
  TextAreaField,
  TextField,
  copyText,
  groupName,
  useControlList,
} from "@noxaaa/prism-oss-web-core/console/shared";
import type {
  AgentMetrics,
  NodeEnrollmentProfile,
  NodeGroup,
  NodeGeoIP,
  NodeResource,
  RegistrationToken,
} from "@noxaaa/prism-oss-web-core/console/types";

const noMetricNodes: NodeResource[] = [];

function nodeGroupIDs(node: NodeResource): string[] {
  return Array.isArray(node.group_ids) ? node.group_ids : [];
}

export function NodesPage({ mode }: { mode: "admin" | "user" }) {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const canManage = hasPermission(session, "nodes.manage") && mode === "admin";
  const canReadMetrics = hasPermission(session, "nodes.read") && hasPermission(session, "traffic.read_all");
  const nodeGroups = useControlList<NodeGroup>("/api/control/node-groups");
  const nodes = useControlList<NodeResource>("/api/control/nodes");
  const enrollmentProfiles = useControlList<NodeEnrollmentProfile>("/api/control/node-enrollment-profiles");
  const metricNodes = canReadMetrics ? nodes.data : noMetricNodes;
  const metricsByNode = useNodeMetrics(canReadMetrics, metricNodes);
  const [nodeGroupCreateOpen, setNodeGroupCreateOpen] = useState(false);
  const [nodeCreateOpen, setNodeCreateOpen] = useState(false);
  const [editingNodeGroup, setEditingNodeGroup] = useState<NodeGroup | null>(null);
  const [deletingNodeGroup, setDeletingNodeGroup] = useState<NodeGroup | null>(null);
  const [editingNode, setEditingNode] = useState<NodeResource | null>(null);
  const [deletingNode, setDeletingNode] = useState<NodeResource | null>(null);
  const [installCommandFallback, setInstallCommandFallback] = useState<{ nodeName: string; command: string } | null>(null);
  const [enrollmentCreateOpen, setEnrollmentCreateOpen] = useState(false);
  const [editingEnrollmentProfile, setEditingEnrollmentProfile] = useState<NodeEnrollmentProfile | null>(null);
  const [viewingEnrollmentProfile, setViewingEnrollmentProfile] = useState<NodeEnrollmentProfile | null>(null);
  const [deletingEnrollmentProfile, setDeletingEnrollmentProfile] = useState<NodeEnrollmentProfile | null>(null);
  const [enrollmentScriptFallback, setEnrollmentScriptFallback] = useState<{ profileName: string; script: string } | null>(null);
  const [enrollmentActionID, setEnrollmentActionID] = useState<string | null>(null);
  const [agentActionNodeID, setAgentActionNodeID] = useState<string | null>(null);

  async function refreshAll() {
    await Promise.all([nodeGroups.refresh(), nodes.refresh(), enrollmentProfiles.refresh()]);
  }

  async function copyInstallCommand(node: NodeResource) {
    try {
      const result = await controlPost<RegistrationToken>(`/api/control/nodes/${node.id}/registration-token`, { ttl_hours: 24 });
      if (!result.install_command) {
        toast.error(t("nodes.installCommandMissing"));
        return;
      }
      try {
        await copyText(result.install_command, t("nodes.installCommandCopied"));
        setInstallCommandFallback(null);
      } catch {
        setInstallCommandFallback({ nodeName: node.name, command: result.install_command });
        toast.error(t("nodes.installCommandCopyFailed"));
      }
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function updateAgentAutoUpdate(node: NodeResource, enabled: boolean) {
    setAgentActionNodeID(node.id);
    try {
      await controlPatch<NodeResource>(`/api/control/nodes/${node.id}/agent-update-policy`, { enabled });
      toast.success(enabled ? t("nodes.agentAutoUpdateEnabled") : t("nodes.agentAutoUpdateDisabled"));
      await nodes.refresh();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setAgentActionNodeID(null);
    }
  }

  async function requestAgentUpgrade(node: NodeResource) {
    setAgentActionNodeID(node.id);
    try {
      await controlPost<NodeResource>(`/api/control/nodes/${node.id}/agent-upgrade`, {});
      toast.success(t("nodes.agentUpgradeQueued"));
      await nodes.refresh();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setAgentActionNodeID(null);
    }
  }

  async function rotateAndCopyEnrollmentScript(profile: NodeEnrollmentProfile) {
    setEnrollmentActionID(profile.id);
    try {
      const result = await controlPost<NodeEnrollmentProfile>(`/api/control/node-enrollment-profiles/${profile.id}/rotate-token`, {});
      const script = result.shell_script || result.install_command;
      if (!script) {
        toast.error(t("nodes.enrollmentScriptMissing"));
        return;
      }
      try {
        await copyText(script, t("nodes.enrollmentScriptCopied"));
        setEnrollmentScriptFallback(null);
      } catch {
        setEnrollmentScriptFallback({ profileName: profile.name, script });
        toast.error(t("nodes.installCommandCopyFailed"));
      }
      await enrollmentProfiles.refresh();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setEnrollmentActionID(null);
    }
  }

  return (
    <PageStack>
      <SummaryGrid>
        <SummaryCard icon={<NetworkIcon />} label={t("nodes.nodeGroups")} loading={nodeGroups.loading} value={nodeGroups.data.length} />
        <SummaryCard icon={<ServerIcon />} label={t("nodes.nodes")} loading={nodes.loading} value={nodes.data.length} />
        <SummaryCard icon={<ActivityIcon />} label={t("nodes.online")} loading={nodes.loading} value={nodes.data.filter((node) => node.status === "ONLINE").length} />
      </SummaryGrid>

      {canManage ? (
        <>
          <NodeGroupCreateDrawer onCreated={refreshAll} onOpenChange={setNodeGroupCreateOpen} open={nodeGroupCreateOpen} />
          <NodeCreateDrawer groups={nodeGroups.data} onCreated={refreshAll} onOpenChange={setNodeCreateOpen} open={nodeCreateOpen} />
          <NodeEnrollmentCreateDrawer groups={nodeGroups.data} onCreated={async (profile) => { await refreshAll(); setEnrollmentScriptFallback(profile.shell_script ? { profileName: profile.name, script: profile.shell_script } : null); }} onOpenChange={setEnrollmentCreateOpen} open={enrollmentCreateOpen} />
          <NodeEnrollmentEditDrawer groups={nodeGroups.data} onOpenChange={(open) => !open && setEditingEnrollmentProfile(null)} onUpdated={refreshAll} profile={editingEnrollmentProfile} />
          <NodeEnrollmentDetailDrawer onOpenChange={(open) => !open && setViewingEnrollmentProfile(null)} profile={viewingEnrollmentProfile} />
          <NodeEnrollmentDeleteDialog onDeleted={refreshAll} onOpenChange={(open) => !open && setDeletingEnrollmentProfile(null)} profile={deletingEnrollmentProfile} />
          <NodeGroupEditDrawer group={editingNodeGroup} onOpenChange={(open) => !open && setEditingNodeGroup(null)} onUpdated={refreshAll} />
          <NodeGroupDeleteDialog group={deletingNodeGroup} onDeleted={refreshAll} onOpenChange={(open) => !open && setDeletingNodeGroup(null)} />
          <NodeEditDrawer groups={nodeGroups.data} node={editingNode} onOpenChange={(open) => !open && setEditingNode(null)} onUpdated={refreshAll} />
          <NodeDeleteDialog node={deletingNode} onDeleted={refreshAll} onOpenChange={(open) => !open && setDeletingNode(null)} />
          <div className="flex flex-wrap justify-end gap-2">
            <Button onClick={() => setNodeGroupCreateOpen(true)} type="button" variant="outline">
              <PlusIcon data-icon="inline-start" />
              {t("nodes.createNodeGroup")}
            </Button>
            <Button onClick={() => setNodeCreateOpen(true)} type="button">
              <PlusIcon data-icon="inline-start" />
              {t("nodes.createNode")}
            </Button>
            <Button onClick={() => setEnrollmentCreateOpen(true)} type="button" variant="outline">
              <PlusIcon data-icon="inline-start" />
              {t("nodes.createEnrollmentProfile")}
            </Button>
          </div>
        </>
      ) : null}

      {installCommandFallback ? (
        <Alert>
          <AlertTitle>{t("nodes.installCommandReady")}</AlertTitle>
          <AlertDescription className="flex flex-col gap-3">
            <span>{t("nodes.installCommandCopyFailedDescription", { name: installCommandFallback.nodeName })}</span>
            <Textarea readOnly value={installCommandFallback.command} />
          </AlertDescription>
        </Alert>
      ) : null}

      {enrollmentScriptFallback ? (
        <Alert>
          <AlertTitle>{t("nodes.enrollmentScriptReady")}</AlertTitle>
          <AlertDescription className="flex flex-col gap-3">
            <span>{t("nodes.enrollmentScriptDescription", { name: enrollmentScriptFallback.profileName })}</span>
            <Textarea readOnly value={enrollmentScriptFallback.script} />
          </AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{t("nodes.nodeGroups")}</CardTitle>
          <CardDescription>{t("nodes.nodeGroupsDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <DataState loading={nodeGroups.loading} loadingFallback={<TableSkeleton columns={canManage ? 4 : 3} rows={4} />} error={nodeGroups.error}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("field.name")}</TableHead>
                  <TableHead>{t("field.description")}</TableHead>
                  <TableHead>{t("nodes.nodes")}</TableHead>
                  {canManage ? <TableHead>{t("common.actions")}</TableHead> : null}
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodeGroups.data.map((group) => (
                  <TableRow key={group.id}>
                    <TableCell>{group.name}</TableCell>
                    <TableCell>{group.description || t("common.none")}</TableCell>
                    <TableCell>{nodes.data.filter((node) => nodeGroupIDs(node).includes(group.id)).length}</TableCell>
                    {canManage ? (
                      <TableCell>
                        <div className="flex flex-wrap gap-2">
                          <Button onClick={() => setEditingNodeGroup(group)} size="sm" type="button" variant="outline">
                            <PencilIcon data-icon="inline-start" />
                            {t("common.edit")}
                          </Button>
                          <Button onClick={() => setDeletingNodeGroup(group)} size="sm" type="button" variant="outline">
                            <Trash2Icon data-icon="inline-start" />
                            {t("common.delete")}
                          </Button>
                        </div>
                      </TableCell>
                    ) : null}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>

      {canManage ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("nodes.enrollmentProfiles")}</CardTitle>
            <CardDescription>{t("nodes.enrollmentProfilesDescription")}</CardDescription>
            <CardAction>
              <Button onClick={() => setEnrollmentCreateOpen(true)} size="sm" type="button">
                <PlusIcon data-icon="inline-start" />
                {t("common.create")}
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent>
            <DataState loading={enrollmentProfiles.loading} loadingFallback={<TableSkeleton columns={7} rows={4} />} error={enrollmentProfiles.error}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("field.name")}</TableHead>
                    <TableHead>{t("overview.status")}</TableHead>
                    <TableHead>{t("nodes.groups")}</TableHead>
                    <TableHead>{t("nodes.uses")}</TableHead>
                    <TableHead>{t("nodes.expiresAt")}</TableHead>
                    <TableHead>{t("nodes.allowedCIDRs")}</TableHead>
                    <TableHead>{t("common.actions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {enrollmentProfiles.data.map((profile) => (
                    <TableRow key={profile.id}>
                      <TableCell>
                        <div className="flex flex-col gap-1">
                          <span className="font-medium">{profile.name}</span>
                          {profile.description ? <span className="text-xs text-muted-foreground">{profile.description}</span> : null}
                        </div>
                      </TableCell>
                      <TableCell><StatusBadge value={profile.enabled ? "ENABLED" : "DISABLED"} /></TableCell>
                      <TableCell>{profile.group_ids.map((id) => groupName(nodeGroups.data, id)).join(", ")}</TableCell>
                      <TableCell>{profile.max_uses > 0 ? `${profile.used_count}/${profile.max_uses}` : String(profile.used_count)}</TableCell>
                      <TableCell>{shortDate(profile.expires_at, locale)}</TableCell>
                      <TableCell>{profile.allowed_cidrs.length ? profile.allowed_cidrs.join(", ") : t("common.none")}</TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-2">
                          <Button disabled={enrollmentActionID === profile.id} onClick={() => void rotateAndCopyEnrollmentScript(profile)} size="sm" type="button" variant="outline">
                            <CopyIcon data-icon="inline-start" />
                            {t("nodes.rotateAndCopyScript")}
                          </Button>
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button aria-label={t("common.actions")} size="icon-sm" type="button" variant="outline">
                                <MoreHorizontalIcon />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="w-52">
                              <DropdownMenuItem onSelect={() => setViewingEnrollmentProfile(profile)}>
                                {t("common.view")}
                              </DropdownMenuItem>
                              <DropdownMenuItem onSelect={() => setEditingEnrollmentProfile(profile)}>
                                <PencilIcon data-icon="inline-start" />
                                {t("common.edit")}
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem onSelect={() => setDeletingEnrollmentProfile(profile)} variant="destructive">
                                <Trash2Icon data-icon="inline-start" />
                                {t("common.delete")}
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </DataState>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{mode === "admin" ? t("nodes.nodes") : t("nodes.availableNodes")}</CardTitle>
          <CardAction>
            <Button onClick={refreshAll} size="sm" type="button" variant="outline">
              <RefreshCwIcon data-icon="inline-start" />
              {t("common.refresh")}
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          <DataState loading={nodes.loading} loadingFallback={<TableSkeleton columns={canManage ? 10 : 9} rows={5} />} error={nodes.error}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("field.name")}</TableHead>
                  <TableHead>{t("overview.status")}</TableHead>
                  <TableHead className="w-24">{t("nodes.country")}</TableHead>
                  <TableHead>{t("nodes.agent")}</TableHead>
                  <TableHead>{t("nodes.groups")}</TableHead>
                  <TableHead>{t("nodes.listenIPs")}</TableHead>
                  <TableHead>{t("nodes.sendIPs")}</TableHead>
                  <TableHead>{t("nodes.ports")}</TableHead>
                  <TableHead>{t("nodes.config")}</TableHead>
                  {canManage ? <TableHead>{t("common.actions")}</TableHead> : null}
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodes.data.map((node) => (
                  <TableRow key={node.id}>
                    <TableCell>
                      <div className="flex flex-col gap-1">
                        <NodeSystemHover metrics={metricsByNode[node.id]} node={node} />
                        {node.public_description ? <span className="text-xs text-muted-foreground">{node.public_description}</span> : null}
                        <span className="text-xs text-muted-foreground">{node.enrollment_profile?.name ? t("nodes.enrolledFrom", { name: node.enrollment_profile.name }) : t("nodes.manualRegistration")}</span>
                      </div>
                    </TableCell>
                    <TableCell><StatusBadge value={node.status} /></TableCell>
                    <TableCell><NodeGeoIPCell geoip={node.geoip} /></TableCell>
                    <TableCell><NodeAgentSummary node={node} /></TableCell>
                    <TableCell>{nodeGroupIDs(node).map((id) => groupName(nodeGroups.data, id)).join(", ")}</TableCell>
                    <TableCell>{node.listen_ips.map((item) => item.listen_ip).join(", ")}</TableCell>
                    <TableCell>{node.send_ips?.length ? node.send_ips.map((item) => item.send_ip).join(", ") : t("rules.defaultSendIP")}</TableCell>
                    <TableCell>
                      <div className="flex flex-col gap-1">
                        <span>{node.port_ranges.map((range) => `${localizeEnum(range.protocol, locale)} ${range.start_port}-${range.end_port}`).join(", ")}</span>
                        <span className="text-xs text-muted-foreground">{t("nodes.maxRulePortsShort", { count: node.max_rule_ports ?? 256 })}</span>
                      </div>
                    </TableCell>
                    <TableCell>{node.applied_config_version}/{node.desired_config_version}</TableCell>
                    {canManage ? (
                      <TableCell>
                        <div className="flex flex-wrap gap-2">
                          <Button onClick={() => copyInstallCommand(node)} size="sm" type="button" variant="outline">
                            <CopyIcon data-icon="inline-start" />
                            {t("nodes.copyInstallCommand")}
                          </Button>
                          <Button onClick={() => setEditingNode(node)} size="sm" type="button" variant="outline">
                            <PencilIcon data-icon="inline-start" />
                            {t("common.edit")}
                          </Button>
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button aria-label={t("common.actions")} size="icon-sm" type="button" variant="outline">
                                <MoreHorizontalIcon />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="w-52">
                              <DropdownMenuCheckboxItem
                                checked={node.agent_auto_update_enabled}
                                disabled={agentActionNodeID === node.id}
                                onCheckedChange={(checked) => updateAgentAutoUpdate(node, checked === true)}
                              >
                                {t("nodes.agentAutoUpdate")}
                              </DropdownMenuCheckboxItem>
                              <DropdownMenuItem disabled={agentActionNodeID === node.id} onSelect={() => void requestAgentUpgrade(node)}>
                                <DownloadIcon data-icon="inline-start" />
                                {t("nodes.upgradeAgent")}
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem onSelect={() => setDeletingNode(node)} variant="destructive">
                                <Trash2Icon data-icon="inline-start" />
                                {t("common.delete")}
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </div>
                      </TableCell>
                    ) : null}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>

      {canReadMetrics ? <NodeMetricsPanel metricsByNode={metricsByNode} nodes={metricNodes} /> : null}
    </PageStack>
  );
}

function useNodeMetrics(canReadMetrics: boolean, nodes: NodeResource[]) {
  const [metricsByNode, setMetricsByNode] = useState<Record<string, AgentMetrics>>({});
  const nodeIDsKey = nodes.map((node) => node.id).sort().join(",");

  useEffect(() => {
    const nodeIDs = new Set(nodeIDsKey ? nodeIDsKey.split(",") : []);
    setMetricsByNode((current) => Object.fromEntries(Object.entries(current).filter(([nodeID]) => nodeIDs.has(nodeID))));
    if (!canReadMetrics || nodeIDs.size === 0) {
      return undefined;
    }

    const source = new EventSource("/api/control/nodes/metrics/stream");
    source.addEventListener("metrics", (event) => {
      const payload = JSON.parse((event as MessageEvent).data) as { node_id?: string; metrics?: AgentMetrics };
      if (!payload.node_id || !payload.metrics || !nodeIDs.has(payload.node_id)) {
        return;
      }
      setMetricsByNode((current) => ({
        ...current,
        [payload.node_id as string]: payload.metrics as AgentMetrics,
      }));
    });

    return () => {
      source.close();
    };
  }, [canReadMetrics, nodeIDsKey]);

  return metricsByNode;
}

function NodeAgentSummary({ node }: { node: NodeResource }) {
  const { locale, t } = useI18n();
  const targetVersion = node.desired_agent_version || t("common.unknown");
  return (
    <div className="flex min-w-44 flex-col gap-1">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-xs">{node.agent_version || t("common.unknown")}</span>
        <StatusBadge value={node.agent_update_status || "IDLE"} />
      </div>
      <span className="text-xs text-muted-foreground">{t("nodes.agentTargetVersion", { version: targetVersion })}</span>
      {node.agent_build_time ? <span className="text-xs text-muted-foreground">{shortDate(node.agent_build_time, locale)}</span> : null}
      {node.agent_update_error ? <span className="max-w-56 text-xs text-destructive">{node.agent_update_error}</span> : null}
    </div>
  );
}

function NodeGeoIPCell({ geoip }: { geoip: NodeGeoIP | undefined }) {
  const { t } = useI18n();
  const code = geoip?.country_code || t("common.unknown");
  const flag = geoip?.flag_emoji || "??";
  return (
    <HoverCard>
      <HoverCardTrigger asChild>
        <span className="inline-flex min-w-16 cursor-default items-center gap-2 whitespace-nowrap">
          <span aria-hidden="true">{flag}</span>
          <span className="font-mono text-xs">{code}</span>
        </span>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-72">
        <MetricDetail label="IP" value={geoip?.ip || t("common.unknown")} />
        <MetricDetail label={t("nodes.geoipSource")} value={geoip?.source || t("common.unknown")} />
        <MetricDetail label={t("nodes.country")} value={geoip?.country_name || code} />
        <a
          className="block pt-2 text-xs text-muted-foreground underline-offset-4 hover:underline"
          href="https://db-ip.com/db/download/ip-to-country-lite"
          rel="noreferrer"
          target="_blank"
        >
          {geoip?.attribution || "DB-IP"}
        </a>
      </HoverCardContent>
    </HoverCard>
  );
}

function NodeSystemHover({ metrics, node }: { metrics: AgentMetrics | undefined; node: NodeResource }) {
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

function virtualizationLabel(metrics: AgentMetrics | undefined, fallback: string) {
  const system = metrics?.virtualization_system?.trim();
  const role = metrics?.virtualization_role?.trim();
  if (system && role) {
    return `${system} (${role})`;
  }
  return system || role || fallback;
}

function NodeAgentDetails({ node }: { node: NodeResource }) {
  const { locale, t } = useI18n();
  return (
    <div className="grid gap-3 rounded-md border p-3 text-sm md:grid-cols-2">
      <div>
        <div className="text-xs text-muted-foreground">{t("nodes.currentAgentVersion")}</div>
        <div className="font-mono">{node.agent_version || t("common.unknown")}</div>
      </div>
      <div>
        <div className="text-xs text-muted-foreground">{t("nodes.targetAgentVersion")}</div>
        <div className="font-mono">{node.desired_agent_version || t("common.unknown")}</div>
      </div>
      <div>
        <div className="text-xs text-muted-foreground">{t("nodes.agentAutoUpdate")}</div>
        <div>{node.agent_auto_update_enabled ? t("common.enabled") : t("common.disabled")}</div>
      </div>
      <div>
        <div className="text-xs text-muted-foreground">{t("nodes.agentUpdateStatus")}</div>
        <StatusBadge value={node.agent_update_status || "IDLE"} />
      </div>
      <div>
        <div className="text-xs text-muted-foreground">{t("nodes.dnsPublishAddress")}</div>
        <div>{formatDNSPublishAddresses(node)}</div>
      </div>
      <div>
        <div className="text-xs text-muted-foreground">{t("nodes.dataplaneMode")}</div>
        <div>{localizeEnum(node.dataplane_mode || "AUTO", locale)}</div>
      </div>
      <div>
        <div className="text-xs text-muted-foreground">{t("nodes.dataplaneStatus")}</div>
        <StatusBadge value={node.dataplane_status || "UNKNOWN"} />
      </div>
      <div>
        <div className="text-xs text-muted-foreground">{t("nodes.dataplaneInstanceID")}</div>
        <div className="break-all font-mono">{node.dataplane_instance_id || t("common.unknown")}</div>
      </div>
      <div>
        <div className="text-xs text-muted-foreground">{t("nodes.dataplaneLastHash")}</div>
        <div className="break-all font-mono">{node.dataplane_last_hash || t("common.unknown")}</div>
      </div>
      {node.dataplane_error ? (
        <div className="md:col-span-2">
          <div className="text-xs text-muted-foreground">{t("nodes.dataplaneError")}</div>
          <div className="break-words text-destructive">{node.dataplane_error}</div>
        </div>
      ) : null}
    </div>
  );
}

function formatDNSPublishAddresses(node: NodeResource) {
  const addresses = node.dns_publish_addresses ?? [];
  if (addresses.length === 0) {
    return "-";
  }
  return addresses
    .filter((address) => address.enabled)
    .map((address) => `${address.address} ${address.source === "AUTO" ? "(auto)" : ""}`.trim())
    .join(", ") || "-";
}

function NodeGroupCreateDrawer({ onCreated, onOpenChange, open }: { onCreated: () => Promise<void>; onOpenChange: (open: boolean) => void; open: boolean }) {
  const { t } = useI18n();
  async function handleCreated() {
    await onCreated();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("nodes.createNodeGroup")}</SheetTitle>
          <SheetDescription>{t("nodes.createNodeGroupDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          <NodeGroupCreateForm onCreated={handleCreated} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function NodeCreateDrawer({ groups, onCreated, onOpenChange, open }: { groups: NodeGroup[]; onCreated: () => Promise<void>; onOpenChange: (open: boolean) => void; open: boolean }) {
  const { t } = useI18n();
  async function handleCreated() {
    await onCreated();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("nodes.createNode")}</SheetTitle>
          <SheetDescription>{t("nodes.createNodeDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          <NodeMutationForm groups={groups} onSaved={handleCreated} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function NodeEditDrawer({ groups, node, onOpenChange, onUpdated }: { groups: NodeGroup[]; node: NodeResource | null; onOpenChange: (open: boolean) => void; onUpdated: () => Promise<void> }) {
  const { t } = useI18n();
  async function handleUpdated() {
    await onUpdated();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={Boolean(node)}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("nodes.editNode")}</SheetTitle>
          <SheetDescription>{t("nodes.editNodeDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          {node ? (
            <div className="flex flex-col gap-5">
              <NodeAgentDetails node={node} />
              <Separator />
              <NodeMutationForm groups={groups} node={node} onSaved={handleUpdated} />
            </div>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function NodeGroupEditDrawer({ group, onOpenChange, onUpdated }: { group: NodeGroup | null; onOpenChange: (open: boolean) => void; onUpdated: () => Promise<void> }) {
  const { t } = useI18n();
  async function handleUpdated() {
    await onUpdated();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={Boolean(group)}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("nodes.editNodeGroup")}</SheetTitle>
          <SheetDescription>{t("nodes.editNodeGroupDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          {group ? <NodeGroupCreateForm group={group} onCreated={handleUpdated} /> : null}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function NodeGroupCreateForm({ group, onCreated }: { group?: NodeGroup; onCreated: () => Promise<void> }) {
  const { locale, t } = useI18n();
  async function saveGroup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    try {
      const payload = {
        name: form.get("name"),
        description: form.get("description"),
      };
      if (group) {
        await controlPatch<NodeGroup>(`/api/control/node-groups/${group.id}`, payload);
      } else {
        await controlPost<NodeGroup>("/api/control/node-groups", payload);
      }
      formElement.reset();
      toast.success(group ? t("nodes.nodeGroupUpdated") : t("nodes.nodeGroupCreated"));
      await onCreated();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={saveGroup}>
      <FieldGroup>
        <TextField defaultValue={group?.name ?? ""} label={t("field.name")} name="name" placeholder={t("nodes.nodeGroupNamePlaceholder")} />
        <TextAreaField defaultValue={group?.description ?? ""} label={t("field.description")} name="description" placeholder={t("nodes.nodeGroupDescriptionPlaceholder")} />
      </FieldGroup>
      <Button type="submit">
        {group ? <PencilIcon data-icon="inline-start" /> : <PlusIcon data-icon="inline-start" />}
        {group ? t("common.save") : t("targets.createGroup")}
      </Button>
    </form>
  );
}

function NodeDeleteDialog({ node, onDeleted, onOpenChange }: { node: NodeResource | null; onDeleted: () => Promise<void>; onOpenChange: (open: boolean) => void }) {
  const { locale, t } = useI18n();

  async function deleteNode() {
    if (!node) {
      return;
    }
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/nodes/${node.id}`);
      toast.success(t("nodes.nodeDeleted"));
      await onDeleted();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={Boolean(node)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("nodes.deleteNode")}</DialogTitle>
          <DialogDescription>{t("nodes.deleteNodeQuestion", { name: node?.name ?? "" })}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} type="button" variant="outline">{t("common.cancel")}</Button>
          <Button onClick={deleteNode} type="button" variant="destructive">{t("common.delete")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function NodeGroupDeleteDialog({ group, onDeleted, onOpenChange }: { group: NodeGroup | null; onDeleted: () => Promise<void>; onOpenChange: (open: boolean) => void }) {
  const { locale, t } = useI18n();

  async function deleteGroup() {
    if (!group) {
      return;
    }
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/node-groups/${group.id}`);
      toast.success(t("nodes.nodeGroupDeleted"));
      await onDeleted();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={Boolean(group)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("nodes.deleteNodeGroup")}</DialogTitle>
          <DialogDescription>{t("nodes.deleteNodeGroupQuestion", { name: group?.name ?? "" })}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} type="button" variant="outline">{t("common.cancel")}</Button>
          <Button onClick={deleteGroup} type="button" variant="destructive">{t("common.delete")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MetricDetail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 py-1">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 break-words text-right font-medium tabular-nums">{value}</span>
    </div>
  );
}
