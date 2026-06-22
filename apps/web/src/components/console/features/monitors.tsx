"use client";

import {
  CopyIcon,
  GlobeIcon,
  HeartPulseIcon,
  PlusIcon,
  RadarIcon,
  RefreshCwIcon,
  Trash2Icon,
} from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { controlDelete, controlPost, shortDate } from "@/components/console/control-api";
import { canUseDNSHealthSelector, dnsPageResourceState } from "@/components/console/dns-page-state";
import { canReadHealthChecks, canUseHealthCheckEditor, healthPageResourceState } from "@/components/console/health-page-state";
import { localizeControlError, useI18n } from "@/components/console/i18n";
import { hasPermission } from "@/components/console/permissions";
import { useConsoleSession } from "@/components/console/shell";
import { DataState, EnumSelect, PageStack, StatusBadge, SummaryCard, SummaryGrid, copyText, useControlList } from "@/components/console/shared";
import type { DNSCredential, DNSRecord, HealthCheck, Monitor, MonitorGroup, RegistrationToken, ResourceOption, Target, TargetGroup } from "@/components/console/types";

export function MonitorsPage() {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const canManage = hasPermission(session, "monitors.manage");
  const monitorGroups = useControlList<MonitorGroup>("/api/control/monitor-groups");
  const monitors = useControlList<Monitor>("/api/control/monitors");
  const [creating, setCreating] = useState(false);

  async function refreshAll() {
    await Promise.all([monitorGroups.refresh(), monitors.refresh()]);
  }

  async function createGroup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreating(true);
    const form = new FormData(event.currentTarget);
    try {
      await controlPost<MonitorGroup>("/api/control/monitor-groups", {
        name: String(form.get("name") ?? ""),
        description: String(form.get("description") ?? ""),
      });
      event.currentTarget.reset();
      toast.success(t("monitors.groupCreated"));
      await refreshAll();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setCreating(false);
    }
  }

  async function createMonitor(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreating(true);
    const form = new FormData(event.currentTarget);
    try {
      await controlPost<Monitor>("/api/control/monitors", {
        name: String(form.get("name") ?? ""),
        group_ids: [String(form.get("group_id") ?? "")].filter(Boolean),
      });
      event.currentTarget.reset();
      toast.success(t("monitors.created"));
      await refreshAll();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setCreating(false);
    }
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

  return (
    <PageStack>
      <SummaryGrid>
        <SummaryCard icon={<RadarIcon />} label={t("monitors.monitors")} value={monitors.data.length} />
        <SummaryCard icon={<RadarIcon />} label={t("monitors.groups")} value={monitorGroups.data.length} />
        <SummaryCard icon={<RadarIcon />} label={t("nodes.online")} value={monitors.data.filter((monitor) => monitor.status === "ONLINE").length} />
      </SummaryGrid>

      {canManage ? (
        <div className="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>{t("monitors.createGroup")}</CardTitle>
            </CardHeader>
            <CardContent>
              <form className="grid gap-4" onSubmit={createGroup}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="monitor-group-name">{t("field.name")}</FieldLabel>
                    <Input id="monitor-group-name" name="name" required />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="monitor-group-description">{t("field.description")}</FieldLabel>
                    <Input id="monitor-group-description" name="description" />
                  </Field>
                </FieldGroup>
                <Button disabled={creating} type="submit"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button>
              </form>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t("monitors.createMonitor")}</CardTitle>
            </CardHeader>
            <CardContent>
              <form className="grid gap-4" onSubmit={createMonitor}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="monitor-name">{t("field.name")}</FieldLabel>
                    <Input id="monitor-name" name="name" required />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="monitor-group-id">{t("monitors.group")}</FieldLabel>
                    <select className="h-9 rounded-md border bg-background px-3 text-sm" id="monitor-group-id" name="group_id" required>
                      <option value="">{t("resource.selectResource")}</option>
                      {monitorGroups.data.map((group) => <option key={group.id} value={group.id}>{group.name}</option>)}
                    </select>
                  </Field>
                </FieldGroup>
                <Button disabled={creating || monitorGroups.data.length === 0} type="submit"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button>
              </form>
            </CardContent>
          </Card>
        </div>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{t("monitors.monitors")}</CardTitle>
          <CardDescription>{t("monitors.description")}</CardDescription>
          <CardAction><Button onClick={refreshAll} size="icon" type="button" variant="outline"><RefreshCwIcon /></Button></CardAction>
        </CardHeader>
        <CardContent>
          <DataState loading={monitors.loading || monitorGroups.loading} error={monitors.error || monitorGroups.error}>
            <Table>
              <TableHeader><TableRow><TableHead>{t("field.name")}</TableHead><TableHead>{t("field.status")}</TableHead><TableHead>{t("monitors.groups")}</TableHead><TableHead>{t("overview.lastSeen")}</TableHead>{canManage ? <TableHead>{t("common.actions")}</TableHead> : null}</TableRow></TableHeader>
              <TableBody>
                {monitors.data.map((monitor) => (
                  <TableRow key={monitor.id}>
                    <TableCell>{monitor.name}</TableCell>
                    <TableCell><StatusBadge value={monitor.status} /></TableCell>
                    <TableCell>{monitor.group_ids.map((id) => monitorGroups.data.find((group) => group.id === id)?.name ?? id).join(", ") || t("common.none")}</TableCell>
                    <TableCell>{shortDate(monitor.last_seen_at, locale)}</TableCell>
                    {canManage ? <TableCell><Button onClick={() => copyInstallCommand(monitor)} size="sm" type="button" variant="outline"><CopyIcon data-icon="inline-start" />{t("nodes.copyInstallCommand")}</Button></TableCell> : null}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>
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
  const [targetScopeType, setTargetScopeType] = useState("TARGETS");
  const [monitorScopeType, setMonitorScopeType] = useState("MONITOR");
  const [probeType, setProbeType] = useState("TCP_PORT");
  const [enabled, setEnabled] = useState(true);
  const resourceState = healthPageResourceState({
    checks,
    targets,
    targetGroups,
    monitors,
    monitorGroups,
    includeEditorDependencies: canUseEditor,
  });

  async function refreshAll() {
    await Promise.all([
      checks.refresh(),
      canUseEditor ? targets.refresh() : Promise.resolve(),
      canUseEditor ? targetGroups.refresh() : Promise.resolve(),
      canUseEditor ? monitors.refresh() : Promise.resolve(),
      canUseEditor ? monitorGroups.refresh() : Promise.resolve(),
    ]);
  }

  async function createCheck(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    try {
      const configText = String(form.get("config") ?? "{}").trim() || "{}";
      await controlPost<HealthCheck>("/api/control/health-checks", {
        name: String(form.get("name") ?? ""),
        probe_type: probeType,
        interval_seconds: Number(form.get("interval_seconds") ?? 30),
        timeout_seconds: Number(form.get("timeout_seconds") ?? 5),
        enabled,
        target_scope: targetScopeType === "TARGETS"
          ? { type: "TARGETS", target_ids: [String(form.get("target_id") ?? "")].filter(Boolean) }
          : { type: "TARGET_GROUP", target_group_id: String(form.get("target_group_id") ?? "") },
        monitor_scope: monitorScopeType === "MONITOR"
          ? { type: "MONITOR", monitor_id: String(form.get("monitor_id") ?? "") }
          : { type: "MONITOR_GROUP", monitor_group_id: String(form.get("monitor_group_id") ?? "") },
        config: JSON.parse(configText),
      });
      event.currentTarget.reset();
      toast.success(t("health.created"));
      await checks.refresh();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
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
        <SummaryCard icon={<HeartPulseIcon />} label={t("health.checks")} value={checks.data.length} />
        <SummaryCard icon={<HeartPulseIcon />} label={t("common.enabled")} value={checks.data.filter((check) => check.enabled).length} />
      </SummaryGrid>
      {canManage && canUseEditor ? (
        <Card>
          <CardHeader><CardTitle>{t("health.create")}</CardTitle></CardHeader>
          <CardContent>
            <form className="grid gap-4 lg:grid-cols-2" onSubmit={createCheck}>
              <Field><FieldLabel htmlFor="health-name">{t("field.name")}</FieldLabel><Input id="health-name" name="name" required /></Field>
              <EnumSelect label={t("health.probeType")} onValueChange={setProbeType} options={[{ value: "TCP_PORT", label: "TCP_PORT" }, { value: "HTTP", label: "HTTP" }, { value: "ICMP", label: "ICMP" }]} value={probeType} />
              <Field><FieldLabel htmlFor="health-interval">{t("health.interval")}</FieldLabel><Input defaultValue="30" id="health-interval" min="1" name="interval_seconds" required type="number" /></Field>
              <Field><FieldLabel htmlFor="health-timeout">{t("health.timeout")}</FieldLabel><Input defaultValue="5" id="health-timeout" min="1" name="timeout_seconds" required type="number" /></Field>
              <EnumSelect label={t("health.targetScope")} onValueChange={setTargetScopeType} options={[{ value: "TARGETS", label: t("targets.targets") }, { value: "TARGET_GROUP", label: t("targets.targetGroup") }]} value={targetScopeType} />
              {targetScopeType === "TARGETS" ? <SelectField label={t("field.target_id")} name="target_id" options={targets.data.map((target) => ({ value: target.id, label: `${target.name} (${target.host}:${target.port})` }))} /> : <SelectField label={t("field.target_group_id")} name="target_group_id" options={targetGroups.data.map((group) => ({ value: group.id, label: group.name }))} />}
              <EnumSelect label={t("health.monitorScope")} onValueChange={setMonitorScopeType} options={[{ value: "MONITOR", label: t("monitors.monitor") }, { value: "MONITOR_GROUP", label: t("monitors.group") }]} value={monitorScopeType} />
              {monitorScopeType === "MONITOR" ? <SelectField label={t("field.monitor_id")} name="monitor_id" options={monitors.data.map((monitor) => ({ value: monitor.id, label: monitor.name }))} /> : <SelectField label={t("field.monitor_group_id")} name="monitor_group_id" options={monitorGroups.data.map((group) => ({ value: group.id, label: group.name }))} />}
              <Field className="lg:col-span-2"><FieldLabel htmlFor="health-config">{t("health.config")}</FieldLabel><Textarea defaultValue="{}" id="health-config" name="config" /></Field>
              <Field orientation="horizontal"><Switch checked={enabled} onCheckedChange={setEnabled} /> <FieldLabel>{t("common.enabled")}</FieldLabel></Field>
              <Button className="lg:col-span-2" type="submit"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button>
            </form>
          </CardContent>
        </Card>
      ) : null}
      <Card>
        <CardHeader><CardTitle>{t("health.checks")}</CardTitle><CardAction><Button onClick={refreshAll} size="icon" type="button" variant="outline"><RefreshCwIcon /></Button></CardAction></CardHeader>
        <CardContent>
          <DataState loading={resourceState.loading} error={resourceState.error}>
            <Table>
              <TableHeader><TableRow><TableHead>{t("field.name")}</TableHead><TableHead>{t("health.probeType")}</TableHead><TableHead>{t("targets.targets")}</TableHead><TableHead>{t("common.enabled")}</TableHead>{canManage ? <TableHead>{t("common.actions")}</TableHead> : null}</TableRow></TableHeader>
              <TableBody>
                {checks.data.map((check) => (
                  <TableRow key={check.id}>
                    <TableCell>{check.name}</TableCell>
                    <TableCell>{check.probe_type}</TableCell>
                    <TableCell>{check.targets.map((target) => `${target.target_name} (${target.target_host}:${target.target_port})`).join(", ")}</TableCell>
                    <TableCell><StatusBadge value={check.enabled ? "ENABLED" : "DISABLED"} /></TableCell>
                    {canManage ? <TableCell><Button onClick={() => deleteCheck(check)} size="sm" type="button" variant="outline"><Trash2Icon data-icon="inline-start" />{t("common.delete")}</Button></TableCell> : null}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>
    </PageStack>
  );
}

export function DNSPage() {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const canManage = hasPermission(session, "dns.manage");
  const canSelectHealthCheck = canUseDNSHealthSelector(session);
  const credentials = useControlList<DNSCredential>("/api/control/dns/credentials");
  const records = useControlList<DNSRecord>("/api/control/dns/records");
  const checks = useControlList<HealthCheck>(canSelectHealthCheck ? "/api/control/health-checks" : "");
  const [eventType, setEventType] = useState("DNS_FAILOVER");
  const resourceState = dnsPageResourceState({
    credentials,
    records,
    healthChecks: checks,
    includeHealthChecks: canSelectHealthCheck,
  });

  async function refreshAll() {
    await Promise.all([credentials.refresh(), records.refresh(), canSelectHealthCheck ? checks.refresh() : Promise.resolve()]);
  }

  async function createCredential(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    try {
      await controlPost<DNSCredential>("/api/control/dns/credentials", { name: String(form.get("name") ?? ""), provider: "CLOUDFLARE", secret: String(form.get("secret") ?? "") });
      event.currentTarget.reset();
      toast.success(t("dns.credentialCreated"));
      await credentials.refresh();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function createRecord(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    try {
      await controlPost<DNSRecord>("/api/control/dns/records", {
        dns_credential_id: String(form.get("dns_credential_id") ?? ""),
        zone: String(form.get("zone") ?? ""),
        record_name: String(form.get("record_name") ?? ""),
        record_type: String(form.get("record_type") ?? "A"),
        desired_values: String(form.get("desired_values") ?? "").split(/\s*,\s*/).filter(Boolean),
        health_check_id: String(form.get("health_check_id") ?? ""),
        event_type: eventType,
        failover_values: String(form.get("failover_values") ?? "").split(/\s*,\s*/).filter(Boolean),
      });
      event.currentTarget.reset();
      toast.success(t("dns.recordCreated"));
      await records.refresh();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function deleteRecord(record: DNSRecord) {
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/dns/records/${record.id}`);
      toast.success(t("dns.recordDeleted"));
      await records.refresh();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <PageStack>
      <SummaryGrid>
        <SummaryCard icon={<GlobeIcon />} label={t("dns.credentials")} value={credentials.data.length} />
        <SummaryCard icon={<GlobeIcon />} label={t("dns.records")} value={records.data.length} />
      </SummaryGrid>
      {canManage ? (
        <div className="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader><CardTitle>{t("dns.createCredential")}</CardTitle></CardHeader>
            <CardContent>
              <form className="grid gap-4" onSubmit={createCredential}>
                <Field><FieldLabel htmlFor="dns-credential-name">{t("field.name")}</FieldLabel><Input id="dns-credential-name" name="name" required /></Field>
                <Field><FieldLabel htmlFor="dns-secret">Cloudflare token</FieldLabel><Input id="dns-secret" name="secret" required type="password" /></Field>
                <Button type="submit"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button>
              </form>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>{t("dns.createRecord")}</CardTitle></CardHeader>
            <CardContent>
              <form className="grid gap-4" onSubmit={createRecord}>
                <SelectField label={t("field.dns_credential_id")} name="dns_credential_id" options={credentials.data.map((credential) => ({ value: credential.id, label: credential.name }))} />
                <Field><FieldLabel htmlFor="dns-zone">{t("dns.zone")}</FieldLabel><Input id="dns-zone" name="zone" required /></Field>
                <Field><FieldLabel htmlFor="dns-record-name">{t("dns.record")}</FieldLabel><Input id="dns-record-name" name="record_name" required /></Field>
                <Field><FieldLabel htmlFor="dns-record-type">{t("dns.type")}</FieldLabel><Input defaultValue="A" id="dns-record-type" name="record_type" required /></Field>
                <Field><FieldLabel htmlFor="dns-values">{t("dns.values")}</FieldLabel><Input id="dns-values" name="desired_values" placeholder="192.0.2.1, 192.0.2.2" required /></Field>
                {canSelectHealthCheck ? (
                  <>
                    <OptionalSelectField label={t("field.health_check_id")} name="health_check_id" options={checks.data.map((check) => ({ value: check.id, label: check.name }))} />
                    <EnumSelect label={t("dns.eventType")} onValueChange={setEventType} options={[{ value: "DNS_FAILOVER", label: "DNS_FAILOVER" }, { value: "DNS_DELETE_OFFLINE", label: "DNS_DELETE_OFFLINE" }, { value: "DNS_DELETE_ALL", label: "DNS_DELETE_ALL" }, { value: "DNS_RESTORE", label: "DNS_RESTORE" }]} value={eventType} />
                    <Field><FieldLabel htmlFor="dns-failover-values">{t("dns.failoverValues")}</FieldLabel><Input id="dns-failover-values" name="failover_values" placeholder="198.51.100.10" /></Field>
                  </>
                ) : null}
                <Button disabled={credentials.data.length === 0} type="submit"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button>
              </form>
            </CardContent>
          </Card>
        </div>
      ) : null}
      <Card>
        <CardHeader><CardTitle>{t("dns.records")}</CardTitle><CardAction><Button onClick={refreshAll} size="icon" type="button" variant="outline"><RefreshCwIcon /></Button></CardAction></CardHeader>
        <CardContent>
          <DataState loading={resourceState.loading} error={resourceState.error}>
            <Table>
              <TableHeader><TableRow><TableHead>{t("dns.record")}</TableHead><TableHead>{t("dns.type")}</TableHead><TableHead>{t("dns.values")}</TableHead><TableHead>{t("dns.credential")}</TableHead>{canManage ? <TableHead>{t("common.actions")}</TableHead> : null}</TableRow></TableHeader>
              <TableBody>
                {records.data.map((record) => (
                  <TableRow key={record.id}>
                    <TableCell>{record.record_name}</TableCell>
                    <TableCell>{record.record_type}</TableCell>
                    <TableCell>{record.desired_values.join(", ")}</TableCell>
                    <TableCell>{credentials.data.find((credential) => credential.id === record.dns_credential_id)?.name ?? record.dns_credential_id}</TableCell>
                    {canManage ? <TableCell><Button onClick={() => deleteRecord(record)} size="sm" type="button" variant="outline"><Trash2Icon data-icon="inline-start" />{t("common.delete")}</Button></TableCell> : null}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>
    </PageStack>
  );
}

function SelectField({ label, name, options }: { label: string; name: string; options: ResourceOption[] }) {
  const { t } = useI18n();
  return (
    <Field>
      <FieldLabel htmlFor={name}>{label}</FieldLabel>
      <select className="h-9 rounded-md border bg-background px-3 text-sm" id={name} name={name} required>
        <option value="">{t("resource.selectResource")}</option>
        {options.map((option) => (
          <option disabled={option.disabled} key={option.value} value={option.value}>{option.label}</option>
        ))}
      </select>
    </Field>
  );
}

function OptionalSelectField({ label, name, options }: { label: string; name: string; options: ResourceOption[] }) {
  const { t } = useI18n();
  return (
    <Field>
      <FieldLabel htmlFor={name}>{label}</FieldLabel>
      <select className="h-9 rounded-md border bg-background px-3 text-sm" id={name} name={name}>
        <option value="">{t("common.none")}</option>
        {options.map((option) => (
          <option disabled={option.disabled} key={option.value} value={option.value}>{option.label}</option>
        ))}
      </select>
    </Field>
  );
}
