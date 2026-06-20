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
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
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
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldSet, FieldLegend } from "@/components/ui/field";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@/components/ui/hover-card";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { bytes, controlDelete, controlPatch, controlPost, formatBitrateBps, shortDate } from "@/components/console/control-api";
import { localizeControlError, localizeEnum, useI18n } from "@/components/console/i18n";
import { hasPermission } from "@/components/console/permissions";
import { ResourceMultiSelect } from "@/components/console/resource-select";
import { useConsoleSession } from "@/components/console/shell";
import {
  DataState,
  EnumSelect,
  PageStack,
  StatusBadge,
  SummaryCard,
  SummaryGrid,
  TextAreaField,
  TextField,
  copyText,
  duration,
  groupName,
  percent,
  useControlList,
} from "@/components/console/shared";
import type {
  AgentMetrics,
  NodeGroup,
  NodeResource,
  RegistrationToken,
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
  const [nodeGroupCreateOpen, setNodeGroupCreateOpen] = useState(false);
  const [nodeCreateOpen, setNodeCreateOpen] = useState(false);
  const [editingNodeGroup, setEditingNodeGroup] = useState<NodeGroup | null>(null);
  const [deletingNodeGroup, setDeletingNodeGroup] = useState<NodeGroup | null>(null);
  const [editingNode, setEditingNode] = useState<NodeResource | null>(null);
  const [deletingNode, setDeletingNode] = useState<NodeResource | null>(null);
  const [installCommandFallback, setInstallCommandFallback] = useState<{ nodeName: string; command: string } | null>(null);
  const [agentActionNodeID, setAgentActionNodeID] = useState<string | null>(null);

  async function refreshAll() {
    await Promise.all([nodeGroups.refresh(), nodes.refresh()]);
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

      <Card>
        <CardHeader>
          <CardTitle>{t("nodes.nodeGroups")}</CardTitle>
          <CardDescription>{t("nodes.nodeGroupsDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <DataState loading={nodeGroups.loading} error={nodeGroups.error}>
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
                    <TableCell>{nodes.data.filter((node) => node.group_ids.includes(group.id)).length}</TableCell>
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
                  <TableHead>{t("nodes.agent")}</TableHead>
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
                    <TableCell><NodeAgentSummary node={node} /></TableCell>
                    <TableCell>{node.group_ids.map((id) => groupName(nodeGroups.data, id)).join(", ")}</TableCell>
                    <TableCell>{node.listen_ips.map((item) => item.listen_ip).join(", ")}</TableCell>
                    <TableCell>{node.port_ranges.map((range) => `${localizeEnum(range.protocol, locale)} ${range.start_port}-${range.end_port}`).join(", ")}</TableCell>
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

      {canReadMetrics ? <NodeMetricsPanel nodes={nodes.data} /> : null}
    </PageStack>
  );
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

function NodeAgentDetails({ node }: { node: NodeResource }) {
  const { t } = useI18n();
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
    </div>
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

function NodeMutationForm({ groups, node, onSaved }: { groups: NodeGroup[]; node?: NodeResource; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const initialProtocol = initialPortProtocol(node);
  const initialPortRange = node?.port_ranges[0];
  const initialStartPort = initialPortRange?.start_port ? String(initialPortRange.start_port) : "";
  const initialEndPort = initialPortRange?.end_port ? String(initialPortRange.end_port) : "";
  const [nodeGroupIDs, setNodeGroupIDs] = useState<string[]>(node?.group_ids ?? []);
  const [protocol, setProtocol] = useState(initialProtocol);
  const [listenIPs, setListenIPs] = useState<Array<{ listen_ip: string; display_name: string }>>(
    node?.listen_ips?.length ? node.listen_ips.map((item) => ({ listen_ip: item.listen_ip, display_name: item.display_name })) : [{ listen_ip: "0.0.0.0", display_name: "default" }],
  );
  const groupOptions = groups.map((group) => ({ value: group.id, label: group.name }));
  const localizedProtocolOptions = protocolOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));

  function updateListenIP(index: number, field: "listen_ip" | "display_name", value: string) {
    setListenIPs((current) => current.map((item, itemIndex) => (itemIndex === index ? { ...item, [field]: value } : item)));
  }

  function addListenIP() {
    setListenIPs((current) => [...current, { listen_ip: "", display_name: "" }]);
  }

  function removeListenIP(index: number) {
    setListenIPs((current) => current.filter((_, itemIndex) => itemIndex !== index));
  }

  async function saveNode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const normalizedListenIPs = listenIPs
      .map((item) => ({ listen_ip: item.listen_ip.trim(), display_name: item.display_name.trim() }))
      .filter((item) => item.listen_ip !== "");
    if (normalizedListenIPs.length === 0) {
      normalizedListenIPs.push({ listen_ip: "0.0.0.0", display_name: "default" });
    }
    const startPortValue = String(form.get("start_port") ?? "");
    const endPortValue = String(form.get("end_port") ?? "");
    const portControlsChanged = protocol !== initialProtocol || startPortValue !== initialStartPort || endPortValue !== initialEndPort;
    const portRanges = node && node.port_ranges.length > 0 && !portControlsChanged
      ? node.port_ranges.map((range) => ({ protocol: range.protocol, start_port: range.start_port, end_port: range.end_port }))
      : nodePortRangesForSelection(protocol, Number(startPortValue || 10000), Number(endPortValue || 20000));
    try {
      const payload = {
        name: form.get("name"),
        public_description: form.get("public_description"),
        group_ids: nodeGroupIDs,
        listen_ips: normalizedListenIPs,
        port_ranges: portRanges,
      };
      if (node) {
        await controlPatch<NodeResource>(`/api/control/nodes/${node.id}`, payload);
      } else {
        await controlPost<NodeResource>("/api/control/nodes", payload);
      }
      formElement.reset();
      setNodeGroupIDs([]);
      toast.success(node ? t("nodes.nodeUpdated") : t("nodes.nodeCreated"));
      await onSaved();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={saveNode}>
      <FieldGroup>
        <TextField defaultValue={node?.name ?? ""} label={t("field.name")} name="name" placeholder="entry-node-a" />
        <ResourceMultiSelect label={t("nodes.nodeGroups")} onValueChange={setNodeGroupIDs} options={groupOptions} values={nodeGroupIDs} />
        <FieldSet>
          <FieldLegend>{t("nodes.listenIPs")}</FieldLegend>
          <FieldDescription>{t("nodes.listenIPsDescription")}</FieldDescription>
          <div className="flex flex-col gap-3">
            {listenIPs.map((item, index) => (
              <div className="grid gap-3 md:grid-cols-[1fr_1fr_auto]" key={index}>
                <Field>
                  <FieldLabel>{t("rules.listenIP")}</FieldLabel>
                  <Input onChange={(event) => updateListenIP(index, "listen_ip", event.target.value)} placeholder="0.0.0.0" value={item.listen_ip} />
                </Field>
                <Field>
                  <FieldLabel>{t("nodes.listenIPLabel")}</FieldLabel>
                  <Input onChange={(event) => updateListenIP(index, "display_name", event.target.value)} placeholder="default" value={item.display_name} />
                </Field>
                <Button className="self-end" disabled={listenIPs.length === 1} onClick={() => removeListenIP(index)} type="button" variant="outline">
                  <Trash2Icon />
                </Button>
              </div>
            ))}
          </div>
          <Button onClick={addListenIP} type="button" variant="outline">
            <PlusIcon data-icon="inline-start" />
            {t("nodes.addListenIP")}
          </Button>
        </FieldSet>
        <EnumSelect label={t("rules.protocol")} onValueChange={setProtocol} options={localizedProtocolOptions} value={protocol} />
        <div className="grid gap-3 md:grid-cols-2">
          <TextField defaultValue={initialStartPort} label={t("nodes.startPort")} name="start_port" placeholder="10000" required={false} type="number" />
          <TextField defaultValue={initialEndPort} label={t("nodes.endPort")} name="end_port" placeholder="20000" required={false} type="number" />
        </div>
        <TextAreaField defaultValue={node?.public_description ?? ""} label={t("nodes.publicDescription")} name="public_description" placeholder="Connect through edge.example.com." />
      </FieldGroup>
      <Button disabled={nodeGroupIDs.length === 0} type="submit">
        {node ? <PencilIcon data-icon="inline-start" /> : <PlusIcon data-icon="inline-start" />}
        {node ? t("common.save") : t("nodes.createNode")}
      </Button>
    </form>
  );
}

function initialPortProtocol(node?: NodeResource) {
  const protocols = new Set((node?.port_ranges ?? []).map((range) => range.protocol));
  if (protocols.has("TCP") && protocols.has("UDP")) {
    return "TCP_UDP";
  }
  return node?.port_ranges[0]?.protocol ?? "TCP";
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
                <TableHead>{t("nodes.bandwidth")}</TableHead>
                <TableHead>CPU</TableHead>
                <TableHead>RAM</TableHead>
                <TableHead>{t("nodes.uptime")}</TableHead>
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
                    <TableCell>
                      <HoverCard>
                        <HoverCardTrigger asChild>
                          <span className="inline-flex cursor-default font-medium">{formatBitrateBps(metrics.bandwidth_bps)}</span>
                        </HoverCardTrigger>
                        <HoverCardContent align="start">
                          <MetricDetail label={t("usage.upload")} value={bytes(metrics.upload_bytes)} />
                          <MetricDetail label={t("usage.download")} value={bytes(metrics.download_bytes)} />
                          <MetricDetail label="TCP" value={metrics.tcp_connections ?? 0} />
                          <MetricDetail label="UDP/s" value={metrics.udp_packets_per_second ?? 0} />
                        </HoverCardContent>
                      </HoverCard>
                    </TableCell>
                    <TableCell><MetricProgress value={metrics.cpu_percent} label={percent(metrics.cpu_percent)} /></TableCell>
                    <TableCell>
                      <HoverCard>
                        <HoverCardTrigger asChild>
                          <div className="inline-flex w-32 cursor-default"><MetricProgress value={ramPercent(metrics)} label={ramLabel(metrics)} /></div>
                        </HoverCardTrigger>
                        <HoverCardContent align="start">
                          <MetricDetail label="RAM" value={ramDetail(metrics)} />
                        </HoverCardContent>
                      </HoverCard>
                    </TableCell>
                    <TableCell>
                      <HoverCard>
                        <HoverCardTrigger asChild>
                          <span className="inline-flex cursor-default">{duration(metrics.uptime_seconds)}</span>
                        </HoverCardTrigger>
                        <HoverCardContent align="start">
                          <MetricDetail label={t("nodes.bootTime")} value={shortDate(metrics.boot_time, locale)} />
                          <MetricDetail label={t("overview.lastSeen")} value={shortDate(metrics.last_seen_at ?? node.last_seen_at, locale)} />
                        </HoverCardContent>
                      </HoverCard>
                    </TableCell>
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

function MetricProgress({ label, value }: { label: string; value: number | undefined }) {
  const normalized = Math.max(0, Math.min(100, value ?? 0));
  return (
    <div className="flex min-w-28 items-center gap-2">
      <Progress className="w-16" value={normalized} />
      <span className="tabular-nums text-xs text-muted-foreground">{label}</span>
    </div>
  );
}

function MetricDetail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 py-1">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium tabular-nums">{value}</span>
    </div>
  );
}

function ramPercent(metrics: AgentMetrics): number | undefined {
  if (!metrics.ram_total_bytes) {
    return undefined;
  }
  return (Math.max(0, metrics.ram_used_bytes ?? 0) / metrics.ram_total_bytes) * 100;
}

function ramLabel(metrics: AgentMetrics): string {
  if (!metrics.ram_total_bytes) {
    return bytes(metrics.ram_used_bytes);
  }
  return percent(ramPercent(metrics));
}

function ramDetail(metrics: AgentMetrics): string {
  const used = bytes(metrics.ram_used_bytes);
  if (!metrics.ram_total_bytes) {
    return used;
  }
  return `${used} / ${bytes(metrics.ram_total_bytes)}`;
}
