"use client";

import {
  CopyIcon,
  Edit3Icon,
  EyeIcon,
  HeartPulseIcon,
  PlusIcon,
  RadarIcon,
  RefreshCwIcon,
  Trash2Icon,
} from "lucide-react";
import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { Button } from "@noxaaa/prism-oss-web-core/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@noxaaa/prism-oss-web-core/ui/card";
import { Field, FieldGroup, FieldLabel } from "@noxaaa/prism-oss-web-core/ui/field";
import { Input } from "@noxaaa/prism-oss-web-core/ui/input";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@noxaaa/prism-oss-web-core/ui/sheet";
import { Switch } from "@noxaaa/prism-oss-web-core/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@noxaaa/prism-oss-web-core/ui/table";
import { ConfirmDeleteDialog } from "@noxaaa/prism-oss-web-core/console/confirm-delete-dialog";
import { controlDelete, controlGet, controlPatch, controlPost, shortDate } from "@noxaaa/prism-oss-web-core/console/control-api";
import { formatHealthCheckTargets } from "@noxaaa/prism-oss-web-core/console/health-check-targets";
import { countHealthResultsByStatus, formatHealthLatencyMs, summarizeHealthResults } from "@noxaaa/prism-oss-web-core/console/health-result-summary";
import { canReadHealthChecks, canUseHealthCheckEditor, healthPageResourceState } from "@noxaaa/prism-oss-web-core/console/health-page-state";
import { localizeControlError, useI18n } from "@noxaaa/prism-oss-web-core/console/i18n";
import { MultiSelectField } from "@noxaaa/prism-oss-web-core/console/multi-select-field";
import { hasPermission } from "@noxaaa/prism-oss-web-core/console/permissions";
import { useConsoleSession } from "@noxaaa/prism-oss-web-core/console/shell";
import { DataState, EnumSelect, FieldRequirementBadge, PageStack, StatusBadge, SummaryCard, SummaryGrid, TableSkeleton, copyText, useControlList } from "@noxaaa/prism-oss-web-core/console/shared";
import type { HealthCheck, HealthResult, Monitor, MonitorGroup, RegistrationToken, ResourceOption, Target, TargetGroup } from "@noxaaa/prism-oss-web-core/console/types";

type DrawerMode = "create" | "edit" | "detail";

export function MonitorsPage() {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const canManage = hasPermission(session, "monitors.manage");
  const monitorGroups = useControlList<MonitorGroup>("/api/control/monitor-groups");
  const monitors = useControlList<Monitor>("/api/control/monitors");
  const [groupDrawer, setGroupDrawer] = useState<{ mode: DrawerMode; group?: MonitorGroup } | null>(null);
  const [monitorDrawer, setMonitorDrawer] = useState<{ mode: DrawerMode; monitor?: Monitor } | null>(null);
  const [deleteRequest, setDeleteRequest] = useState<{ kind: "group"; item: MonitorGroup } | { kind: "monitor"; item: Monitor } | null>(null);

  async function refreshAll() {
    await Promise.all([monitorGroups.refresh(), monitors.refresh()]);
  }

  async function copyInstallCommand(monitor: Monitor) {
    try {
      const token = await controlPost<RegistrationToken>(`/api/control/monitors/${monitor.id}/registration-token`, { ttl_hours: 24 });
      if (!token.install_command) {
        toast.error(t("nodes.installCommandMissing"));
        return;
      }
      await copyText(token.install_command, t("nodes.installCommandCopied"));
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function deleteGroup(group: MonitorGroup) {
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/monitor-groups/${group.id}`);
      toast.success(t("common.deleted"));
      await refreshAll();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function deleteMonitor(monitor: Monitor) {
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/monitors/${monitor.id}`);
      toast.success(t("common.deleted"));
      await refreshAll();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <PageStack>
      <SummaryGrid>
        <SummaryCard icon={<RadarIcon />} label={t("monitors.monitors")} loading={monitors.loading} value={monitors.data.length} />
        <SummaryCard icon={<RadarIcon />} label={t("monitors.groups")} loading={monitorGroups.loading} value={monitorGroups.data.length} />
        <SummaryCard icon={<RadarIcon />} label={t("nodes.online")} loading={monitors.loading} value={monitors.data.filter((monitor) => monitor.status === "ONLINE").length} />
      </SummaryGrid>

      <Card>
        <CardHeader>
          <CardTitle>{t("monitors.groups")}</CardTitle>
          <CardAction className="flex gap-2">
            {canManage ? <Button onClick={() => setGroupDrawer({ mode: "create" })} size="sm" type="button"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button> : null}
            <Button onClick={refreshAll} size="icon" type="button" variant="outline"><RefreshCwIcon /></Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          <DataState loading={monitorGroups.loading} loadingFallback={<TableSkeleton columns={canManage ? 4 : 3} rows={4} />} error={monitorGroups.error}>
            <Table>
              <TableHeader><TableRow><TableHead>{t("field.name")}</TableHead><TableHead>{t("field.description")}</TableHead><TableHead>{t("monitors.monitors")}</TableHead>{canManage ? <TableHead>{t("common.actions")}</TableHead> : null}</TableRow></TableHeader>
              <TableBody>
                {monitorGroups.data.map((group) => {
                  const members = monitors.data.filter((monitor) => monitorGroupIDs(monitor).includes(group.id));
                  return (
                    <TableRow key={group.id}>
                      <TableCell>{group.name}</TableCell>
                      <TableCell>{group.description || t("common.none")}</TableCell>
                      <TableCell>{members.length}</TableCell>
                      {canManage ? (
                        <TableCell className="flex gap-2">
                          <Button onClick={() => setGroupDrawer({ mode: "detail", group })} size="icon-sm" type="button" variant="outline"><EyeIcon /></Button>
                          <Button onClick={() => setGroupDrawer({ mode: "edit", group })} size="icon-sm" type="button" variant="outline"><Edit3Icon /></Button>
                          <Button onClick={() => setDeleteRequest({ kind: "group", item: group })} size="icon-sm" type="button" variant="outline"><Trash2Icon /></Button>
                        </TableCell>
                      ) : null}
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("monitors.monitors")}</CardTitle>
          <CardDescription>{t("monitors.description")}</CardDescription>
          <CardAction className="flex gap-2">
            {canManage ? <Button disabled={monitorGroups.data.length === 0} onClick={() => setMonitorDrawer({ mode: "create" })} size="sm" type="button"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button> : null}
            <Button onClick={refreshAll} size="icon" type="button" variant="outline"><RefreshCwIcon /></Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          <DataState loading={monitors.loading || monitorGroups.loading} loadingFallback={<TableSkeleton columns={canManage ? 5 : 4} rows={5} />} error={monitors.error || monitorGroups.error}>
            <Table>
              <TableHeader><TableRow><TableHead>{t("field.name")}</TableHead><TableHead>{t("field.status")}</TableHead><TableHead>{t("monitors.groups")}</TableHead><TableHead>{t("overview.lastSeen")}</TableHead>{canManage ? <TableHead>{t("common.actions")}</TableHead> : null}</TableRow></TableHeader>
              <TableBody>
                {monitors.data.map((monitor) => (
                  <TableRow key={monitor.id}>
                    <TableCell>{monitor.name}</TableCell>
                    <TableCell><StatusBadge value={monitor.status} /></TableCell>
                    <TableCell>{monitorGroupIDs(monitor).map((id) => monitorGroups.data.find((group) => group.id === id)?.name ?? id).join(", ") || t("common.none")}</TableCell>
                    <TableCell>{shortDate(monitor.last_seen_at, locale)}</TableCell>
                    {canManage ? (
                      <TableCell className="flex gap-2">
                        <Button onClick={() => copyInstallCommand(monitor)} size="icon-sm" type="button" variant="outline"><CopyIcon /></Button>
                        <Button onClick={() => setMonitorDrawer({ mode: "detail", monitor })} size="icon-sm" type="button" variant="outline"><EyeIcon /></Button>
                        <Button onClick={() => setMonitorDrawer({ mode: "edit", monitor })} size="icon-sm" type="button" variant="outline"><Edit3Icon /></Button>
                        <Button onClick={() => setDeleteRequest({ kind: "monitor", item: monitor })} size="icon-sm" type="button" variant="outline"><Trash2Icon /></Button>
                      </TableCell>
                    ) : null}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>

      <MonitorGroupCreateDrawer open={groupDrawer?.mode === "create"} onOpenChange={(open) => !open && setGroupDrawer(null)} onSaved={refreshAll} />
      <MonitorGroupEditDrawer group={groupDrawer?.mode === "edit" ? groupDrawer.group : undefined} onOpenChange={(open) => !open && setGroupDrawer(null)} onSaved={refreshAll} />
      <MonitorGroupDetailDrawer group={groupDrawer?.mode === "detail" ? groupDrawer.group : undefined} monitors={monitors.data} onOpenChange={(open) => !open && setGroupDrawer(null)} />
      <MonitorCreateDrawer groups={monitorGroups.data} open={monitorDrawer?.mode === "create"} onOpenChange={(open) => !open && setMonitorDrawer(null)} onSaved={refreshAll} />
      <MonitorEditDrawer groups={monitorGroups.data} monitor={monitorDrawer?.mode === "edit" ? monitorDrawer.monitor : undefined} onOpenChange={(open) => !open && setMonitorDrawer(null)} onSaved={refreshAll} />
      <MonitorDetailDrawer groups={monitorGroups.data} monitor={monitorDrawer?.mode === "detail" ? monitorDrawer.monitor : undefined} onOpenChange={(open) => !open && setMonitorDrawer(null)} />
      <ConfirmDeleteDialog
        label={deleteRequest ? dnsDeleteLabel(deleteRequest) : ""}
        open={Boolean(deleteRequest)}
        onConfirm={async () => {
          if (deleteRequest?.kind === "group") await deleteGroup(deleteRequest.item);
          if (deleteRequest?.kind === "monitor") await deleteMonitor(deleteRequest.item);
          setDeleteRequest(null);
        }}
        onOpenChange={(open) => !open && setDeleteRequest(null)}
      />
    </PageStack>
  );
}

export function HealthChecksPage() {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const canManage = hasPermission(session, "health_checks.manage");
  const canRead = canReadHealthChecks(session);
  const canUseEditor = canUseHealthCheckEditor(session);
  const checks = useControlList<HealthCheck>(canRead ? "/api/control/health-checks" : "");
  const targets = useControlList<Target>(canUseEditor ? "/api/control/targets" : "");
  const targetGroups = useControlList<TargetGroup>(canUseEditor ? "/api/control/target-groups" : "");
  const monitors = useControlList<Monitor>(canUseEditor ? "/api/control/monitors" : "");
  const monitorGroups = useControlList<MonitorGroup>(canUseEditor ? "/api/control/monitor-groups" : "");
  const [drawer, setDrawer] = useState<{ mode: DrawerMode; check?: HealthCheck } | null>(null);
  const [deleteRequest, setDeleteRequest] = useState<HealthCheck | null>(null);
  const showActions = canRead || canUseEditor || canManage;
  const resourceState = healthPageResourceState({ checks, targets, targetGroups, monitors, monitorGroups, includeEditorDependencies: canUseEditor });

  async function refreshAll() {
    await Promise.all([
      checks.refresh(),
      canUseEditor ? targets.refresh() : Promise.resolve(),
      canUseEditor ? targetGroups.refresh() : Promise.resolve(),
      canUseEditor ? monitors.refresh() : Promise.resolve(),
      canUseEditor ? monitorGroups.refresh() : Promise.resolve(),
    ]);
  }

  async function deleteCheck(check: HealthCheck) {
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/health-checks/${check.id}`);
      toast.success(t("health.deleted"));
      await checks.refresh();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <PageStack>
      <SummaryGrid>
        <SummaryCard icon={<HeartPulseIcon />} label={t("health.checks")} loading={checks.loading} value={checks.data.length} />
        <SummaryCard icon={<HeartPulseIcon />} label={t("common.enabled")} loading={checks.loading} value={checks.data.filter((check) => check.enabled).length} />
      </SummaryGrid>
      <Card>
        <CardHeader>
          <CardTitle>{t("health.checks")}</CardTitle>
          <CardAction className="flex gap-2">
            {canManage && canUseEditor ? <Button onClick={() => setDrawer({ mode: "create" })} size="sm" type="button"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button> : null}
            <Button onClick={refreshAll} size="icon" type="button" variant="outline"><RefreshCwIcon /></Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          <DataState loading={resourceState.loading} loadingFallback={<TableSkeleton columns={showActions ? 6 : 5} rows={5} />} error={resourceState.error}>
            <Table>
              <TableHeader><TableRow><TableHead>{t("field.name")}</TableHead><TableHead>{t("health.probeType")}</TableHead><TableHead>{t("targets.targets")}</TableHead><TableHead>{t("health.latestResult")}</TableHead><TableHead>{t("common.enabled")}</TableHead>{showActions ? <TableHead>{t("common.actions")}</TableHead> : null}</TableRow></TableHeader>
              <TableBody>
                {checks.data.map((check) => (
                  <TableRow key={check.id}>
                    <TableCell>{check.name}</TableCell>
                    <TableCell>{check.probe_type}</TableCell>
                    <TableCell>{formatHealthCheckTargets(check.targets, targetGroups.data, check.target_scope)}</TableCell>
                    <TableCell><LatestHealthSummary check={check} locale={locale} /></TableCell>
                    <TableCell><StatusBadge value={check.enabled ? "ENABLED" : "DISABLED"} /></TableCell>
                    {showActions ? (
                      <TableCell className="flex gap-2">
                        {canRead ? <Button onClick={() => setDrawer({ mode: "detail", check })} size="icon-sm" type="button" variant="outline"><EyeIcon /></Button> : null}
                        {canUseEditor ? <Button onClick={() => setDrawer({ mode: "edit", check })} size="icon-sm" type="button" variant="outline"><Edit3Icon /></Button> : null}
                        {canManage ? <Button onClick={() => setDeleteRequest(check)} size="icon-sm" type="button" variant="outline"><Trash2Icon /></Button> : null}
                      </TableCell>
                    ) : null}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>
      <HealthCheckCreateDrawer checks={checks.data} targets={targets.data} targetGroups={targetGroups.data} monitors={monitors.data} monitorGroups={monitorGroups.data} open={drawer?.mode === "create"} onOpenChange={(open) => !open && setDrawer(null)} onSaved={checks.refresh} />
      <HealthCheckEditDrawer check={drawer?.mode === "edit" ? drawer.check : undefined} targets={targets.data} targetGroups={targetGroups.data} monitors={monitors.data} monitorGroups={monitorGroups.data} onOpenChange={(open) => !open && setDrawer(null)} onSaved={checks.refresh} />
      <HealthCheckDetailDrawer check={drawer?.mode === "detail" ? drawer.check : undefined} onOpenChange={(open) => !open && setDrawer(null)} />
      <ConfirmDeleteDialog
        label={deleteRequest?.name ?? ""}
        open={Boolean(deleteRequest)}
        onConfirm={async () => {
          if (deleteRequest) await deleteCheck(deleteRequest);
          setDeleteRequest(null);
        }}
        onOpenChange={(open) => !open && setDeleteRequest(null)}
      />
    </PageStack>
  );
}

function MonitorGroupCreateDrawer({ open, onOpenChange, onSaved }: { open: boolean; onOpenChange: (open: boolean) => void; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    setSaving(true);
    try {
      await controlPost<MonitorGroup>("/api/control/monitor-groups", { name: String(form.get("name") ?? ""), description: String(form.get("description") ?? "") });
      formElement.reset();
      toast.success(t("monitors.groupCreated"));
      await onSaved();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <MonitorGroupDrawerShell title={t("monitors.createGroup")} open={open} onOpenChange={onOpenChange}><MonitorGroupForm saving={saving} onSubmit={submit} /></MonitorGroupDrawerShell>;
}

function MonitorGroupEditDrawer({ group, onOpenChange, onSaved }: { group?: MonitorGroup; onOpenChange: (open: boolean) => void; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!group) return;
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    setSaving(true);
    try {
      await controlPatch<MonitorGroup>(`/api/control/monitor-groups/${group.id}`, { name: String(form.get("name") ?? ""), description: String(form.get("description") ?? "") });
      toast.success(t("common.saved"));
      await onSaved();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <MonitorGroupDrawerShell title={t("common.edit")} open={Boolean(group)} onOpenChange={onOpenChange}><MonitorGroupForm group={group} saving={saving} onSubmit={submit} /></MonitorGroupDrawerShell>;
}

function MonitorGroupDetailDrawer({ group, monitors, onOpenChange }: { group?: MonitorGroup; monitors: Monitor[]; onOpenChange: (open: boolean) => void }) {
  const { locale, t } = useI18n();
  const members = monitors.filter((monitor) => group && monitorGroupIDs(monitor).includes(group.id));
  return (
    <MonitorGroupDrawerShell title={group?.name ?? t("monitors.group")} open={Boolean(group)} onOpenChange={onOpenChange}>
      <div className="grid gap-3 px-4 text-sm">
        <DetailRow label={t("field.description")} value={group?.description || t("common.none")} />
        <div className="grid gap-2">
          {members.map((monitor) => <DetailRow key={monitor.id} label={monitor.name} value={`${monitor.status} · ${shortDate(monitor.last_seen_at, locale)}`} />)}
          {members.length === 0 ? <p className="text-muted-foreground">{t("common.none")}</p> : null}
        </div>
      </div>
    </MonitorGroupDrawerShell>
  );
}

function monitorGroupIDs(monitor: Monitor): string[] {
  return Array.isArray(monitor.group_ids) ? monitor.group_ids : [];
}

function MonitorCreateDrawer({ groups, open, onOpenChange, onSaved }: { groups: MonitorGroup[]; open: boolean; onOpenChange: (open: boolean) => void; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    setSaving(true);
    try {
      await controlPost<Monitor>("/api/control/monitors", { name: String(form.get("name") ?? ""), group_ids: form.getAll("group_id").map(String).filter(Boolean) });
      formElement.reset();
      toast.success(t("monitors.created"));
      await onSaved();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <MonitorDrawerShell title={t("monitors.createMonitor")} open={open} onOpenChange={onOpenChange}><MonitorForm groups={groups} saving={saving} onSubmit={submit} /></MonitorDrawerShell>;
}

function MonitorEditDrawer({ groups, monitor, onOpenChange, onSaved }: { groups: MonitorGroup[]; monitor?: Monitor; onOpenChange: (open: boolean) => void; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!monitor) return;
    const form = new FormData(event.currentTarget);
    setSaving(true);
    try {
      await controlPatch<Monitor>(`/api/control/monitors/${monitor.id}`, { name: String(form.get("name") ?? ""), group_ids: form.getAll("group_id").map(String).filter(Boolean) });
      toast.success(t("common.saved"));
      await onSaved();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <MonitorDrawerShell title={t("common.edit")} open={Boolean(monitor)} onOpenChange={onOpenChange}><MonitorForm groups={groups} monitor={monitor} saving={saving} onSubmit={submit} /></MonitorDrawerShell>;
}

function MonitorDetailDrawer({ groups, monitor, onOpenChange }: { groups: MonitorGroup[]; monitor?: Monitor; onOpenChange: (open: boolean) => void }) {
  const { locale, t } = useI18n();
  return (
    <MonitorDrawerShell title={monitor?.name ?? t("monitors.monitor")} open={Boolean(monitor)} onOpenChange={onOpenChange}>
      <div className="grid gap-3 px-4 text-sm">
        <DetailRow label={t("field.status")} value={monitor?.status ?? ""} />
        <DetailRow label={t("monitors.groups")} value={monitor ? monitorGroupIDs(monitor).map((id) => groups.find((group) => group.id === id)?.name ?? id).join(", ") || t("common.none") : t("common.none")} />
        <DetailRow label={t("overview.lastSeen")} value={shortDate(monitor?.last_seen_at, locale)} />
        <DetailRow label={t("nodes.config")} value={`${monitor?.applied_config_version ?? 0}/${monitor?.desired_config_version ?? 0}`} />
      </div>
    </MonitorDrawerShell>
  );
}

function HealthCheckCreateDrawer(props: HealthCheckDrawerProps & { checks: HealthCheck[]; open: boolean }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    setSaving(true);
    try {
      await controlPost<HealthCheck>("/api/control/health-checks", healthCheckPayloadFromForm(formElement));
      formElement.reset();
      toast.success(t("health.created"));
      await props.onSaved();
      props.onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <HealthDrawerShell title={t("health.create")} open={props.open} onOpenChange={props.onOpenChange}><HealthCheckForm key="create" {...props} saving={saving} onSubmit={submit} /></HealthDrawerShell>;
}

function HealthCheckEditDrawer(props: HealthCheckDrawerProps & { check?: HealthCheck }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!props.check) return;
    const formElement = event.currentTarget;
    setSaving(true);
    try {
      await controlPatch<HealthCheck>(`/api/control/health-checks/${props.check.id}`, healthCheckPayloadFromForm(formElement));
      toast.success(t("common.saved"));
      await props.onSaved();
      props.onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <HealthDrawerShell title={t("common.edit")} open={Boolean(props.check)} onOpenChange={props.onOpenChange}><HealthCheckForm key={props.check?.id ?? "empty"} {...props} saving={saving} onSubmit={submit} /></HealthDrawerShell>;
}

function HealthCheckDetailDrawer({ check, onOpenChange }: { check?: HealthCheck; onOpenChange: (open: boolean) => void }) {
  const { locale, t } = useI18n();
  const [results, setResults] = useState<HealthResult[]>([]);
  useEffect(() => {
    let active = true;
    setResults([]);
    if (!check) return () => { active = false; };
    const checkID = check.id;
    controlGet<HealthResult[]>(`/api/control/health-checks/${checkID}/results`).then((next) => {
      if (active) setResults(next);
    }).catch(() => {
      if (active) setResults([]);
    });
    return () => { active = false; };
  }, [check?.id]);
  return (
    <HealthDrawerShell title={check?.name ?? t("health.checks")} open={Boolean(check)} onOpenChange={onOpenChange}>
      <div className="grid gap-4 px-4 text-sm">
        <DetailRow label={t("health.probeType")} value={check?.probe_type ?? ""} />
        <DetailRow label={t("common.enabled")} value={check?.enabled ? t("common.enabled") : t("common.disabled")} />
        <div className="grid gap-2">
          <p className="font-medium">{t("health.latestResult")}</p>
          {(results.length ? results : check?.latest_results ?? []).slice(0, 12).map((result) => (
            <div className="rounded-md border p-2" key={result.id}>
              <div className="flex items-center justify-between"><StatusBadge value={result.status} /><span className="text-muted-foreground">{shortDate(result.observed_at, locale)}</span></div>
              <p className="text-muted-foreground">{formatHealthLatencyMs(result.latency_ms, t("common.none"))}{result.error_message ? ` · ${result.error_message}` : ""}</p>
            </div>
          ))}
          {(results.length === 0 && (check?.latest_results?.length ?? 0) === 0) ? <p className="text-muted-foreground">{t("health.notRun")}</p> : null}
        </div>
      </div>
    </HealthDrawerShell>
  );
}

function MonitorGroupForm({ group, saving, onSubmit }: { group?: MonitorGroup; saving: boolean; onSubmit: (event: FormEvent<HTMLFormElement>) => void }) {
  const { t } = useI18n();
  return (
    <form className="grid gap-4 px-4" onSubmit={onSubmit}>
      <FieldGroup>
        <Field><FieldLabel htmlFor="monitor-group-name">{t("field.name")}<FieldRequirementBadge required /></FieldLabel><Input defaultValue={group?.name ?? ""} id="monitor-group-name" name="name" required /></Field>
        <Field><FieldLabel htmlFor="monitor-group-description">{t("field.description")}<FieldRequirementBadge required={false} /></FieldLabel><Input defaultValue={group?.description ?? ""} id="monitor-group-description" name="description" /></Field>
      </FieldGroup>
      <Button disabled={saving} type="submit">{t("common.save")}</Button>
    </form>
  );
}
function MonitorForm({ groups, monitor, saving, onSubmit }: { groups: MonitorGroup[]; monitor?: Monitor; saving: boolean; onSubmit: (event: FormEvent<HTMLFormElement>) => void }) {
  const { t } = useI18n();
  return (
    <form className="grid gap-4 px-4" onSubmit={onSubmit}>
      <Field><FieldLabel htmlFor="monitor-name">{t("field.name")}<FieldRequirementBadge required /></FieldLabel><Input defaultValue={monitor?.name ?? ""} id="monitor-name" name="name" required /></Field>
      <MultiSelectField defaultValues={monitor?.group_ids ?? []} label={t("monitors.groups")} name="group_id" options={groups.map((group) => ({ value: group.id, label: group.name }))} />
      <Button disabled={saving || (!monitor && groups.length === 0)} type="submit">{t("common.save")}</Button>
    </form>
  );
}
type HealthCheckDrawerProps = {
  targets: Target[];
  targetGroups: TargetGroup[];
  monitors: Monitor[];
  monitorGroups: MonitorGroup[];
  onOpenChange: (open: boolean) => void;
  onSaved: () => Promise<void>;
};
function HealthCheckForm({ check, targets, targetGroups, monitors, monitorGroups, saving, onSubmit }: HealthCheckDrawerProps & { check?: HealthCheck; saving: boolean; onSubmit: (event: FormEvent<HTMLFormElement>) => void }) {
  const { t } = useI18n();
  const [targetScopeType, setTargetScopeType] = useState(check?.target_scope?.type === "TARGET_GROUP" ? "TARGET_GROUP" : "TARGETS");
  const [monitorScopeType, setMonitorScopeType] = useState(check?.monitor_scopes[0]?.scope_type === "MONITOR_GROUP" ? "MONITOR_GROUP" : "MONITOR");
  const [probeType, setProbeType] = useState(check?.probe_type ?? "TCP_PORT");
  const [enabled, setEnabled] = useState(check?.enabled ?? true);
  const directTargets = check?.targets.filter((target) => target.scope_type === "TARGET") ?? [];
  const directTargetIDs = directTargets.map((target) => target.target_id).filter(Boolean);
  const defaultTargetIDs = directTargetIDs.length > 0 ? directTargetIDs : check?.target_scope?.target_ids ?? [];
  return (
    <form className="grid gap-4 px-4" onSubmit={onSubmit}>
      <Field><FieldLabel htmlFor="health-name">{t("field.name")}<FieldRequirementBadge required /></FieldLabel><Input defaultValue={check?.name ?? ""} id="health-name" name="name" required /></Field>
      <EnumSelect label={t("health.probeType")} onValueChange={setProbeType} options={[{ value: "TCP_PORT", label: "TCP_PORT" }, { value: "HTTP", label: "HTTP" }, { value: "ICMP", label: "ICMP" }]} value={probeType} />
      <input name="probe_type" type="hidden" value={probeType} />
      <Field><FieldLabel htmlFor="health-interval">{t("health.interval")}<FieldRequirementBadge required /></FieldLabel><Input defaultValue={String(check?.interval_seconds ?? 30)} id="health-interval" min="1" name="interval_seconds" required type="number" /></Field>
      <Field><FieldLabel htmlFor="health-timeout">{t("health.timeout")}<FieldRequirementBadge required /></FieldLabel><Input defaultValue={String(check?.timeout_seconds ?? 5)} id="health-timeout" min="1" name="timeout_seconds" required type="number" /></Field>
      <EnumSelect label={t("health.targetScope")} onValueChange={setTargetScopeType} options={[{ value: "TARGETS", label: t("targets.targets") }, { value: "TARGET_GROUP", label: t("targets.targetGroup") }]} value={targetScopeType} />
      <input name="target_scope_type" type="hidden" value={targetScopeType} />
      {targetScopeType === "TARGETS" ? (
        <MultiSelectField defaultValues={defaultTargetIDs} label={t("targets.targets")} name="target_id" options={targets.map((target) => ({ value: target.id, label: `${target.name} (${target.host}:${target.port})` }))} />
      ) : <SelectField defaultValue={check?.target_scope?.target_group_id ?? check?.targets.find((target) => target.target_group_id)?.target_group_id ?? ""} label={t("field.target_group_id")} name="target_group_id" options={targetGroups.map((group) => ({ value: group.id, label: group.name }))} />}
      <EnumSelect label={t("health.monitorScope")} onValueChange={setMonitorScopeType} options={[{ value: "MONITOR", label: t("monitors.monitor") }, { value: "MONITOR_GROUP", label: t("monitors.group") }]} value={monitorScopeType} />
      <input name="monitor_scope_type" type="hidden" value={monitorScopeType} />
      {monitorScopeType === "MONITOR" ? <SelectField defaultValue={check?.monitor_scopes[0]?.monitor_id ?? ""} label={t("field.monitor_id")} name="monitor_id" options={monitors.map((monitor) => ({ value: monitor.id, label: monitor.name }))} /> : <SelectField defaultValue={check?.monitor_scopes[0]?.monitor_group_id ?? ""} label={t("field.monitor_group_id")} name="monitor_group_id" options={monitorGroups.map((group) => ({ value: group.id, label: group.name }))} />}
      <HealthProbeConfigFields config={check?.config ?? {}} probeType={probeType} />
      <Field orientation="horizontal"><Switch checked={enabled} onCheckedChange={setEnabled} /> <FieldLabel>{t("common.enabled")}</FieldLabel></Field>
      <input name="enabled" type="hidden" value={enabled ? "true" : "false"} />
      <Button disabled={saving} type="submit">{t("common.save")}</Button>
    </form>
  );
}

function HealthProbeConfigFields({ config, probeType }: { config: Record<string, unknown>; probeType: string }) {
  const { t } = useI18n();
  const normalized = probeType.trim().toUpperCase();
  if (normalized === "ICMP") {
    return (
      <Field>
        <FieldLabel>{t("health.config")}<FieldRequirementBadge required={false} /></FieldLabel>
        <p className="text-sm text-muted-foreground">{t("health.noProbeConfig")}</p>
      </Field>
    );
  }
  return (
    <FieldGroup>
      <FieldLabel>{t("health.config")}<FieldRequirementBadge required={false} /></FieldLabel>
      {(normalized === "TCP_PORT" || normalized === "HTTP") ? (
        <Field>
          <FieldLabel htmlFor="health-config-port">{t("health.portOverride")}<FieldRequirementBadge required={false} /></FieldLabel>
          <Input defaultValue={probeConfigNumber(config, "port_override")} id="health-config-port" max="65535" min="1" name="config_port_override" placeholder="443" type="number" />
        </Field>
      ) : null}
      {normalized === "HTTP" ? (
        <div className="grid gap-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="health-config-scheme">{t("health.httpScheme")}<FieldRequirementBadge required={false} /></FieldLabel>
              <select className="h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm" defaultValue={healthProbeSchemeDefault(config)} id="health-config-scheme" name="config_http_scheme">
                <option value="http">http</option>
                <option value="https">https</option>
              </select>
            </Field>
            <Field>
              <FieldLabel htmlFor="health-config-method">{t("health.httpMethod")}<FieldRequirementBadge required={false} /></FieldLabel>
              <Input defaultValue={probeConfigString(config, "method", "GET")} id="health-config-method" name="config_http_method" placeholder="GET" />
            </Field>
          </div>
          <Field>
            <FieldLabel htmlFor="health-config-path">{t("health.httpPath")}<FieldRequirementBadge required={false} /></FieldLabel>
            <Input defaultValue={probeConfigString(config, "path", "/")} id="health-config-path" name="config_http_path" placeholder="/" />
          </Field>
          <Field>
            <FieldLabel htmlFor="health-config-statuses">{t("health.expectedStatuses")}<FieldRequirementBadge required={false} /></FieldLabel>
            <Input defaultValue={probeConfigStatusesText(config)} id="health-config-statuses" name="config_http_expected_statuses" placeholder="200, 204, 301" />
          </Field>
        </div>
      ) : null}
    </FieldGroup>
  );
}

function LatestHealthSummary({ check, locale }: { check: HealthCheck; locale: Parameters<typeof shortDate>[1] }) {
  const { t } = useI18n();
  const results = check.latest_results ?? [];
  const latest = summarizeHealthResults(results);
  if (!latest) {
    return <span className="text-muted-foreground">{t("health.notRun")}</span>;
  }
  const statusLabel = results.length > 1 ? `${latest.status} ${countHealthResultsByStatus(results, latest.status)}/${results.length}` : latest.status;
  return <span>{statusLabel} · {formatHealthLatencyMs(latest.latency_ms, t("common.none"))} · {shortDate(latest.observed_at, locale)}</span>;
}

function MonitorGroupDrawerShell({ title, open, onOpenChange, children }: DrawerShellProps) {
  return <BaseDrawer title={title} description="" open={open} onOpenChange={onOpenChange}>{children}</BaseDrawer>;
}

function MonitorDrawerShell({ title, open, onOpenChange, children }: DrawerShellProps) {
  return <BaseDrawer title={title} description="" open={open} onOpenChange={onOpenChange}>{children}</BaseDrawer>;
}

function HealthDrawerShell({ title, open, onOpenChange, children }: DrawerShellProps) {
  return <BaseDrawer title={title} description="" open={open} onOpenChange={onOpenChange}>{children}</BaseDrawer>;
}


type DrawerShellProps = {
  title: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
};

function BaseDrawer({ title, description, open, onOpenChange, children }: DrawerShellProps & { description: string }) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          {description ? <SheetDescription>{description}</SheetDescription> : null}
        </SheetHeader>
        {children}
      </SheetContent>
    </Sheet>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return <div className="grid gap-1"><span className="text-muted-foreground">{label}</span><span className="break-words">{value}</span></div>;
}

function SelectField({ label, name, options, defaultValue, value, onChange }: { label: string; name: string; options: ResourceOption[]; defaultValue?: string; value?: string; onChange?: (value: string) => void }) {
  const { t } = useI18n();
  return (
    <Field>
      <FieldLabel htmlFor={name}>{label}<FieldRequirementBadge required /></FieldLabel>
      <select className="h-9 rounded-md border bg-background px-3 text-sm" defaultValue={value === undefined ? defaultValue : undefined} id={name} name={name} onChange={(event) => onChange?.(event.currentTarget.value)} required value={value}>
        <option value="">{t("resource.selectResource")}</option>
        {options.map((option) => <option disabled={option.disabled} key={option.value} value={option.value}>{option.label}</option>)}
      </select>
    </Field>
  );
}

function OptionalSelectField({ label, name, options }: { label: string; name: string; options: ResourceOption[] }) {
  const { t } = useI18n();
  return (
    <Field>
      <FieldLabel htmlFor={name}>{label}<FieldRequirementBadge required={false} /></FieldLabel>
      <select className="h-9 rounded-md border bg-background px-3 text-sm" id={name} name={name}>
        <option value="">{t("common.none")}</option>
        {options.map((option) => <option disabled={option.disabled} key={option.value} value={option.value}>{option.label}</option>)}
      </select>
    </Field>
  );
}

function healthCheckPayloadFromForm(formElement: HTMLFormElement) {
  const form = new FormData(formElement);
  const targetScopeType = String(form.get("target_scope_type") ?? "TARGETS");
  const monitorScopeType = String(form.get("monitor_scope_type") ?? "MONITOR");
  return {
    name: String(form.get("name") ?? ""),
    probe_type: String(form.get("probe_type") ?? "TCP_PORT"),
    interval_seconds: Number(form.get("interval_seconds") ?? 30),
    timeout_seconds: Number(form.get("timeout_seconds") ?? 5),
    enabled: String(form.get("enabled") ?? "true") === "true",
    target_scope: targetScopeType === "TARGET_GROUP"
      ? { type: "TARGET_GROUP", target_group_id: String(form.get("target_group_id") ?? "") }
      : { type: "TARGETS", target_ids: form.getAll("target_id").map(String).filter(Boolean) },
    monitor_scope: monitorScopeType === "MONITOR_GROUP"
      ? { type: "MONITOR_GROUP", monitor_group_id: String(form.get("monitor_group_id") ?? "") }
      : { type: "MONITOR", monitor_id: String(form.get("monitor_id") ?? "") },
    config: healthProbeConfigFromForm(form, String(form.get("probe_type") ?? "TCP_PORT")),
  };
}

export function healthProbeConfigFromForm(form: FormData, probeType: string): Record<string, unknown> {
  const normalized = probeType.trim().toUpperCase();
  const config: Record<string, unknown> = {};
  if (normalized === "TCP_PORT" || normalized === "HTTP") {
    const port = parseOptionalInteger(form.get("config_port_override"));
    if (port >= 1 && port <= 65535) config.port_override = port;
  }
  if (normalized === "HTTP") {
    const scheme = String(form.get("config_http_scheme") ?? "http").trim().toLowerCase();
    config.scheme = scheme === "https" ? "https" : "http";
    const method = String(form.get("config_http_method") ?? "GET").trim();
    config.method = method || "GET";
    const path = String(form.get("config_http_path") ?? "/").trim();
    config.path = path.startsWith("/") ? path : `/${path || ""}`;
    const statuses = parseExpectedStatuses(String(form.get("config_http_expected_statuses") ?? ""));
    if (statuses.length > 0) config.expected_statuses = statuses;
  }
  return config;
}

function parseOptionalInteger(value: FormDataEntryValue | null): number {
  const raw = String(value ?? "").trim();
  if (!raw) return 0;
  const parsed = Number(raw);
  return Number.isInteger(parsed) ? parsed : 0;
}

function parseExpectedStatuses(raw: string): number[] {
  const seen = new Set<number>();
  return raw.split(/[\s,]+/)
    .map((value) => Number(value.trim()))
    .filter((value) => Number.isInteger(value) && value >= 100 && value <= 599)
    .filter((value) => {
      if (seen.has(value)) return false;
      seen.add(value);
      return true;
    });
}

function probeConfigString(config: Record<string, unknown>, key: string, fallback: string): string {
  const value = config[key];
  return typeof value === "string" && value.trim() ? value.trim() : fallback;
}

export function healthProbeSchemeDefault(config: Record<string, unknown>): "http" | "https" {
  return probeConfigString(config, "scheme", "http").toLowerCase() === "https" ? "https" : "http";
}

function probeConfigNumber(config: Record<string, unknown>, key: string): string {
  const value = config[key];
  return typeof value === "number" && Number.isFinite(value) ? String(value) : "";
}

function probeConfigStatusesText(config: Record<string, unknown>): string {
  const value = config.expected_statuses;
  return Array.isArray(value) ? value.filter((item) => typeof item === "number" && Number.isFinite(item)).join(", ") : "";
}

function dnsDeleteLabel(request: { kind: "group"; item: MonitorGroup } | { kind: "monitor"; item: Monitor }) {
  return request.item.name;
}
