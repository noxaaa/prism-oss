"use client";

import {
  ActivityIcon,
  CopyIcon,
  DownloadIcon,
  FileJsonIcon,
  MoreHorizontalIcon,
  NetworkIcon,
  PlusIcon,
  RadarIcon,
  RefreshCwIcon,
  RouteIcon,
  ServerIcon,
  ShieldIcon,
  TargetIcon,
  UploadIcon,
  UsersIcon,
} from "lucide-react";
import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldSet, FieldLegend } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { bytes, controlDelete, controlGet, controlPatch, controlPost, optionLabel, shortDate } from "@/components/console/control-api";
import { localizeControlError, localizeEnum, useI18n } from "@/components/console/i18n";
import { hasPermission } from "@/components/console/permissions";
import { ResourceMultiSelect, ResourceSelect } from "@/components/console/resource-select";
import { useConsoleSession } from "@/components/console/shell";
import {
  ControlledTextField,
  DataState,
  EnumSelect,
  PageStack,
  ResourceTable,
  StatusBadge,
  SummaryCard,
  SummaryGrid,
  TextAreaField,
  TextField,
  copyText,
  duration,
  ensureFirstValue,
  groupName,
  monitorGroupName,
  percent,
  useControlList,
} from "@/components/console/shared";
import { cn } from "@/lib/utils";
import type {
  AgentMetrics,
  Member,
  Monitor,
  MonitorGroup,
  NodeGroup,
  NodeResource,
  RegistrationToken,
  ResourceOption,
  Role,
  Rule,
  RuleExportPayload,
  RuleImportResult,
  RuleTraffic,
  Target,
  TargetGroup,
} from "@/components/console/types";

const protocolOptions = [
  { value: "TCP", label: "TCP" },
  { value: "UDP", label: "UDP" },
  { value: "TCP_UDP", label: "TCP + UDP" },
];

function nodePortRangesForSelection(protocol: string, startPort: number, endPort: number) {
  const protocols = protocol === "TCP_UDP" ? ["TCP", "UDP"] : [protocol];
  return protocols.map((rangeProtocol) => ({
    protocol: rangeProtocol,
    start_port: startPort,
    end_port: endPort,
  }));
}

export function NodesPage({ mode }: { mode: "admin" | "user" }) {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const canManage = hasPermission(session, "nodes.manage") && mode === "admin";
  const canReadMetrics = hasPermission(session, "nodes.read") && hasPermission(session, "traffic.read_all");
  const nodeGroups = useControlList<NodeGroup>("/api/control/node-groups");
  const nodes = useControlList<NodeResource>("/api/control/nodes");
  const [token, setToken] = useState<RegistrationToken | null>(null);
  const [nodeGroupCreateOpen, setNodeGroupCreateOpen] = useState(false);
  const [nodeCreateOpen, setNodeCreateOpen] = useState(false);

  async function refreshAll() {
    await Promise.all([nodeGroups.refresh(), nodes.refresh()]);
  }

  async function createToken(nodeID: string) {
    try {
      const result = await controlPost<RegistrationToken>(`/api/control/nodes/${nodeID}/registration-token`, { ttl_hours: 24 });
      setToken(result);
      toast.success(t("nodes.registrationTokenCreated"));
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <PageStack>
      <SummaryGrid>
        <SummaryCard icon={<NetworkIcon />} label={t("nodes.nodeGroups")} value={nodeGroups.data.length} />
        <SummaryCard icon={<ServerIcon />} label={t("nodes.nodes")} value={nodes.data.length} />
        <SummaryCard icon={<ActivityIcon />} label={t("nodes.online")} value={nodes.data.filter((node) => node.status === "ONLINE").length} />
      </SummaryGrid>

      {canManage ? (
        <>
          <NodeGroupCreateDrawer onCreated={refreshAll} onOpenChange={setNodeGroupCreateOpen} open={nodeGroupCreateOpen} />
          <NodeCreateDrawer groups={nodeGroups.data} onCreated={refreshAll} onOpenChange={setNodeCreateOpen} open={nodeCreateOpen} />
          <div className="flex flex-wrap justify-end gap-2">
            <Button onClick={() => setNodeGroupCreateOpen(true)} type="button" variant="outline">
              <PlusIcon data-icon="inline-start" />
              {t("nodes.createNodeGroup")}
            </Button>
            <Button onClick={() => setNodeCreateOpen(true)} type="button">
              <PlusIcon data-icon="inline-start" />
              {t("nodes.createNode")}
            </Button>
          </div>
        </>
      ) : null}

      {token?.install_command ? (
        <Alert>
          <FileJsonIcon />
          <AlertTitle>{t("nodes.installCommand")}</AlertTitle>
          <AlertDescription>
            <code className="block overflow-auto rounded-lg bg-muted p-3 text-xs">{token.install_command}</code>
            <Button className="mt-3" onClick={() => copyText(token.install_command ?? "", t("common.copied"))} size="sm" type="button" variant="outline">
              <CopyIcon data-icon="inline-start" />
              {t("common.copy")}
            </Button>
          </AlertDescription>
        </Alert>
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
          <DataState loading={nodes.loading} error={nodes.error}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("field.name")}</TableHead>
                  <TableHead>{t("overview.status")}</TableHead>
                  <TableHead>{t("nodes.groups")}</TableHead>
                  <TableHead>{t("nodes.listenIPs")}</TableHead>
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
                        <span>{node.name}</span>
                        {node.public_description ? <span className="text-xs text-muted-foreground">{node.public_description}</span> : null}
                      </div>
                    </TableCell>
                    <TableCell><StatusBadge value={node.status} /></TableCell>
                    <TableCell>{node.group_ids.map((id) => groupName(nodeGroups.data, id)).join(", ")}</TableCell>
                    <TableCell>{node.listen_ips.map((item) => item.listen_ip).join(", ")}</TableCell>
                    <TableCell>{node.port_ranges.map((range) => `${localizeEnum(range.protocol, locale)} ${range.start_port}-${range.end_port}`).join(", ")}</TableCell>
                    <TableCell>{node.applied_config_version}/{node.desired_config_version}</TableCell>
                    {canManage ? (
                      <TableCell>
                        <Button onClick={() => createToken(node.id)} size="sm" type="button" variant="outline">{t("nodes.token")}</Button>
                      </TableCell>
                    ) : null}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>

      {canReadMetrics ? <NodeMetricsPanel nodes={nodes.data} /> : null}
    </PageStack>
  );
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
          <NodeCreateForm groups={groups} onCreated={handleCreated} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function NodeGroupCreateForm({ onCreated }: { onCreated: () => Promise<void> }) {
  const { locale, t } = useI18n();
  async function createGroup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    try {
      await controlPost<NodeGroup>("/api/control/node-groups", {
        name: form.get("name"),
        description: form.get("description"),
      });
      formElement.reset();
      toast.success(t("nodes.nodeGroupCreated"));
      await onCreated();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={createGroup}>
      <FieldGroup>
        <TextField label={t("field.name")} name="name" placeholder={t("nodes.nodeGroupNamePlaceholder")} />
        <TextAreaField label={t("field.description")} name="description" placeholder={t("nodes.nodeGroupDescriptionPlaceholder")} />
      </FieldGroup>
      <Button type="submit">
        <PlusIcon data-icon="inline-start" />
        {t("targets.createGroup")}
      </Button>
    </form>
  );
}

function NodeCreateForm({ groups, onCreated }: { groups: NodeGroup[]; onCreated: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const [nodeGroupIDs, setNodeGroupIDs] = useState<string[]>([]);
  const [protocol, setProtocol] = useState("TCP");
  const groupOptions = groups.map((group) => ({ value: group.id, label: group.name }));
  const localizedProtocolOptions = protocolOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));

  async function createNode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    try {
      await controlPost<NodeResource>("/api/control/nodes", {
        name: form.get("name"),
        public_description: form.get("public_description"),
        group_ids: nodeGroupIDs,
        listen_ips: [{ listen_ip: form.get("listen_ip"), display_name: form.get("display_name") }],
        port_ranges: nodePortRangesForSelection(protocol, Number(form.get("start_port")), Number(form.get("end_port"))),
      });
      formElement.reset();
      setNodeGroupIDs([]);
      toast.success(t("nodes.nodeCreated"));
      await onCreated();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={createNode}>
      <FieldGroup>
        <TextField label={t("field.name")} name="name" placeholder="entry-node-a" />
        <ResourceMultiSelect label={t("nodes.nodeGroups")} onValueChange={setNodeGroupIDs} options={groupOptions} values={nodeGroupIDs} />
        <TextField label={t("rules.listenIP")} name="listen_ip" placeholder="0.0.0.0" />
        <TextField label={t("nodes.listenIPLabel")} name="display_name" placeholder="default" />
        <EnumSelect label={t("rules.protocol")} onValueChange={setProtocol} options={localizedProtocolOptions} value={protocol} />
        <div className="grid gap-3 md:grid-cols-2">
          <TextField label={t("nodes.startPort")} name="start_port" placeholder="10000" type="number" />
          <TextField label={t("nodes.endPort")} name="end_port" placeholder="20000" type="number" />
        </div>
        <TextAreaField label={t("nodes.publicDescription")} name="public_description" placeholder="Connect through edge.example.com." />
      </FieldGroup>
      <Button disabled={nodeGroupIDs.length === 0} type="submit">
        <PlusIcon data-icon="inline-start" />
        {t("nodes.createNode")}
      </Button>
    </form>
  );
}

function NodeMetricsPanel({ nodes }: { nodes: NodeResource[] }) {
  const { locale, t } = useI18n();
  const [metricsByNode, setMetricsByNode] = useState<Record<string, AgentMetrics>>({});

  useEffect(() => {
    const nodeIDs = new Set(nodes.map((node) => node.id));
    setMetricsByNode((current) => Object.fromEntries(Object.entries(current).filter(([nodeID]) => nodeIDs.has(nodeID))));
    if (nodes.length === 0) {
      return undefined;
    }

    const sources = nodes.map((node) => {
      const source = new EventSource(`/api/control/nodes/${node.id}/metrics/stream`);
      source.addEventListener("metrics", (event) => {
        setMetricsByNode((current) => ({
          ...current,
          [node.id]: JSON.parse((event as MessageEvent).data) as AgentMetrics,
        }));
      });
      return source;
    });

    return () => {
      sources.forEach((source) => source.close());
    };
  }, [nodes]);

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
                <TableHead>TCP</TableHead>
                <TableHead>UDP/s</TableHead>
                <TableHead>{t("nodes.bandwidth")}</TableHead>
                <TableHead>CPU</TableHead>
                <TableHead>RAM</TableHead>
                <TableHead>{t("usage.upload")}</TableHead>
                <TableHead>{t("usage.download")}</TableHead>
                <TableHead>{t("nodes.uptime")}</TableHead>
                <TableHead>{t("nodes.bootTime")}</TableHead>
                <TableHead>{t("overview.lastSeen")}</TableHead>
                <TableHead>{t("nodes.config")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {nodes.map((node) => {
                const metrics = metricsByNode[node.id] ?? {};
                return (
                  <TableRow key={node.id}>
                    <TableCell>
                      <div className="flex flex-col gap-1">
                        <span>{node.name}</span>
                        {node.public_description ? <span className="text-xs text-muted-foreground">{node.public_description}</span> : null}
                      </div>
                    </TableCell>
                    <TableCell><StatusBadge value={metrics.status ?? node.status} /></TableCell>
                    <TableCell>{metrics.tcp_connections ?? 0}</TableCell>
                    <TableCell>{metrics.udp_packets_per_second ?? 0}</TableCell>
                    <TableCell>{metrics.bandwidth_bps ?? 0} bps</TableCell>
                    <TableCell>{percent(metrics.cpu_percent)}</TableCell>
                    <TableCell>{bytes(metrics.ram_used_bytes)}</TableCell>
                    <TableCell>{bytes(metrics.upload_bytes)}</TableCell>
                    <TableCell>{bytes(metrics.download_bytes)}</TableCell>
                    <TableCell>{duration(metrics.uptime_seconds)}</TableCell>
                    <TableCell>{shortDate(metrics.boot_time, locale)}</TableCell>
                    <TableCell>{shortDate(metrics.last_seen_at ?? node.last_seen_at, locale)}</TableCell>
                    <TableCell>{metrics.applied_config_version ?? node.applied_config_version}/{metrics.desired_config_version ?? node.desired_config_version}</TableCell>
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
