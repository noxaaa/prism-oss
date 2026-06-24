"use client";

import { Edit3Icon, EyeIcon, GlobeIcon, PlusIcon, RefreshCwIcon, Trash2Icon } from "lucide-react";
import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { ConfirmDeleteDialog } from "@/components/console/confirm-delete-dialog";
import { controlDelete, controlPatch, controlPost, shortDate } from "@/components/console/control-api";
import { localizeControlError, useI18n } from "@/components/console/i18n";
import { MultiSelectField } from "@/components/console/multi-select-field";
import { hasPermission } from "@/components/console/permissions";
import { useConsoleSession } from "@/components/console/shell";
import { DataState, EnumSelect, PageStack, StatusBadge, SummaryCard, SummaryGrid, TableSkeleton, useControlList } from "@/components/console/shared";
import type { DNSCredential, DNSInstance, DNSManagedRecord, NotificationChannel, ResourceOption } from "@/components/console/types";

type DrawerMode = "create" | "edit" | "detail";
type DeleteRequest =
  | { kind: "credential"; item: DNSCredential }
  | { kind: "managed_record"; item: DNSManagedRecord }
  | { kind: "instance"; item: DNSInstance }
  | { kind: "channel"; item: NotificationChannel };

export function DNSPage() {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const canManage = hasPermission(session, "dns.manage");
  const credentials = useControlList<DNSCredential>("/api/control/dns/credentials");
  const managedRecords = useControlList<DNSManagedRecord>("/api/control/dns/managed-records");
  const instances = useControlList<DNSInstance>("/api/control/dns/instances");
  const notificationChannels = useControlList<NotificationChannel>("/api/control/notification-channels");
  const nodeGroups = useControlList<ResourceOption>("/api/control/resource-options/node-groups?access=USE");
  const [credentialDrawer, setCredentialDrawer] = useState<{ mode: DrawerMode; credential?: DNSCredential } | null>(null);
  const [managedRecordDrawer, setManagedRecordDrawer] = useState<{ mode: DrawerMode; record?: DNSManagedRecord } | null>(null);
  const [instanceDrawer, setInstanceDrawer] = useState<{ mode: DrawerMode; instance?: DNSInstance } | null>(null);
  const [channelDrawer, setChannelDrawer] = useState<{ mode: DrawerMode; channel?: NotificationChannel } | null>(null);
  const [deleteRequest, setDeleteRequest] = useState<DeleteRequest | null>(null);
  const resourceLoading = credentials.loading || managedRecords.loading || instances.loading || notificationChannels.loading;
  const resourceError = credentials.error || managedRecords.error || instances.error || notificationChannels.error;
  const instanceLoading = resourceLoading || nodeGroups.loading;
  const instanceError = resourceError || nodeGroups.error;
  const activeCredential = credentialDrawer?.credential ? credentials.data.find((credential) => credential.id === credentialDrawer.credential?.id) ?? credentialDrawer.credential : undefined;

  async function refreshAll() {
    await Promise.all([credentials.refresh(), managedRecords.refresh(), instances.refresh(), notificationChannels.refresh(), nodeGroups.refresh()]);
  }

  async function deleteCredential(credential: DNSCredential) {
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/dns/credentials/${credential.id}`);
      toast.success(t("common.deleted"));
      await refreshAll();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function deleteManagedRecord(record: DNSManagedRecord) {
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/dns/managed-records/${record.id}`);
      toast.success(t("dns.recordDeleted"));
      await refreshAll();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function deleteInstance(instance: DNSInstance) {
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/dns/instances/${instance.id}`);
      toast.success(t("common.deleted"));
      await refreshAll();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function deleteChannel(channel: NotificationChannel) {
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/notification-channels/${channel.id}`);
      toast.success(t("common.deleted"));
      await refreshAll();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function evaluateRecord(record: DNSManagedRecord) {
    try {
      await controlPost<DNSManagedRecord>(`/api/control/dns/managed-records/${record.id}/evaluate`, {});
      toast.success(t("common.saved"));
      await refreshAll();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <PageStack>
      <SummaryGrid>
        <SummaryCard icon={<GlobeIcon />} label={t("dns.credentials")} loading={credentials.loading} value={credentials.data.length} />
        <SummaryCard icon={<GlobeIcon />} label={t("dns.managedRecords")} loading={managedRecords.loading} value={managedRecords.data.length} />
        <SummaryCard icon={<GlobeIcon />} label={t("dns.instances")} loading={instances.loading} value={instances.data.length} />
      </SummaryGrid>
      <CredentialTable canManage={canManage} credentials={credentials.data} error={credentials.error} loading={credentials.loading} onDelete={(item) => setDeleteRequest({ kind: "credential", item })} onEdit={(credential) => setCredentialDrawer({ mode: "edit", credential })} onDetail={(credential) => setCredentialDrawer({ mode: "detail", credential })} onNew={() => setCredentialDrawer({ mode: "create" })} onRefresh={refreshAll} />
      <ManagedRecordTable canManage={canManage} error={resourceError} instances={instances.data} loading={resourceLoading} records={managedRecords.data} onDelete={(item) => setDeleteRequest({ kind: "managed_record", item })} onDetail={(record) => setManagedRecordDrawer({ mode: "detail", record })} onEdit={(record) => setManagedRecordDrawer({ mode: "edit", record })} onEvaluate={evaluateRecord} onNew={() => setManagedRecordDrawer({ mode: "create" })} onRefresh={refreshAll} />
      <InstanceTable canManage={canManage} error={instanceError} instances={instances.data} loading={instanceLoading} records={managedRecords.data} onDelete={(item) => setDeleteRequest({ kind: "instance", item })} onDetail={(instance) => setInstanceDrawer({ mode: "detail", instance })} onEdit={(instance) => setInstanceDrawer({ mode: "edit", instance })} onNew={() => setInstanceDrawer({ mode: "create" })} onRefresh={refreshAll} />
      <NotificationChannelTable canManage={canManage} channels={notificationChannels.data} error={resourceError} loading={resourceLoading} onDelete={(item) => setDeleteRequest({ kind: "channel", item })} onDetail={(channel) => setChannelDrawer({ mode: "detail", channel })} onEdit={(channel) => setChannelDrawer({ mode: "edit", channel })} onNew={() => setChannelDrawer({ mode: "create" })} onRefresh={refreshAll} />

      <DNSCredentialCreateDrawer open={credentialDrawer?.mode === "create"} onOpenChange={(open) => !open && setCredentialDrawer(null)} onSaved={credentials.refresh} />
      <DNSCredentialEditDrawer credential={credentialDrawer?.mode === "edit" ? activeCredential : undefined} onOpenChange={(open) => !open && setCredentialDrawer(null)} onSaved={credentials.refresh} />
      <DNSCredentialDetailDrawer credential={credentialDrawer?.mode === "detail" ? activeCredential : undefined} onOpenChange={(open) => !open && setCredentialDrawer(null)} onSaved={credentials.refresh} />
      <DNSManagedRecordCreateDrawer credentials={credentials.data} open={managedRecordDrawer?.mode === "create"} onOpenChange={(open) => !open && setManagedRecordDrawer(null)} onSaved={refreshAll} />
      <DNSManagedRecordEditDrawer credentials={credentials.data} record={managedRecordDrawer?.mode === "edit" ? managedRecordDrawer.record : undefined} onOpenChange={(open) => !open && setManagedRecordDrawer(null)} onSaved={refreshAll} />
      <DNSManagedRecordDetailDrawer record={managedRecordDrawer?.mode === "detail" ? managedRecordDrawer.record : undefined} instances={instances.data} onOpenChange={(open) => !open && setManagedRecordDrawer(null)} />
      <DNSInstanceCreateDrawer channels={notificationChannels.data} instances={instances.data} managedRecords={managedRecords.data} nodeGroups={nodeGroups.data} open={instanceDrawer?.mode === "create"} onOpenChange={(open) => !open && setInstanceDrawer(null)} onSaved={refreshAll} />
      <DNSInstanceEditDrawer channels={notificationChannels.data} instance={instanceDrawer?.mode === "edit" ? instanceDrawer.instance : undefined} instances={instances.data} managedRecords={managedRecords.data} nodeGroups={nodeGroups.data} onOpenChange={(open) => !open && setInstanceDrawer(null)} onSaved={refreshAll} />
      <DNSInstanceDetailDrawer instance={instanceDrawer?.mode === "detail" ? instanceDrawer.instance : undefined} managedRecords={managedRecords.data} onOpenChange={(open) => !open && setInstanceDrawer(null)} />
      <NotificationChannelCreateDrawer open={channelDrawer?.mode === "create"} onOpenChange={(open) => !open && setChannelDrawer(null)} onSaved={refreshAll} />
      <NotificationChannelEditDrawer channel={channelDrawer?.mode === "edit" ? channelDrawer.channel : undefined} onOpenChange={(open) => !open && setChannelDrawer(null)} onSaved={refreshAll} />
      <NotificationChannelDetailDrawer channel={channelDrawer?.mode === "detail" ? channelDrawer.channel : undefined} onOpenChange={(open) => !open && setChannelDrawer(null)} />
      <ConfirmDeleteDialog
        label={deleteRequest ? dnsDeleteLabel(deleteRequest) : ""}
        open={Boolean(deleteRequest)}
        onConfirm={async () => {
          if (deleteRequest?.kind === "credential") await deleteCredential(deleteRequest.item);
          if (deleteRequest?.kind === "managed_record") await deleteManagedRecord(deleteRequest.item);
          if (deleteRequest?.kind === "instance") await deleteInstance(deleteRequest.item);
          if (deleteRequest?.kind === "channel") await deleteChannel(deleteRequest.item);
          setDeleteRequest(null);
        }}
        onOpenChange={(open) => !open && setDeleteRequest(null)}
      />
    </PageStack>
  );
}

function CredentialTable(props: { canManage: boolean; credentials: DNSCredential[]; loading: boolean; error: string; onNew: () => void; onRefresh: () => void; onDetail: (credential: DNSCredential) => void; onEdit: (credential: DNSCredential) => void; onDelete: (credential: DNSCredential) => void }) {
  const { t } = useI18n();
  return (
    <Card>
      <CardHeader><CardTitle>{t("dns.credentials")}</CardTitle><CardAction className="flex gap-2">{props.canManage ? <Button onClick={props.onNew} size="sm" type="button"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button> : null}<Button onClick={props.onRefresh} size="icon" type="button" variant="outline"><RefreshCwIcon /></Button></CardAction></CardHeader>
      <CardContent><DataState loading={props.loading} loadingFallback={<TableSkeleton columns={props.canManage ? 4 : 3} rows={4} />} error={props.error}><Table><TableHeader><TableRow><TableHead>{t("field.name")}</TableHead><TableHead>Provider</TableHead><TableHead>{t("dns.zone")}</TableHead>{props.canManage ? <TableHead>{t("common.actions")}</TableHead> : null}</TableRow></TableHeader><TableBody>{props.credentials.map((credential) => <TableRow key={credential.id}><TableCell>{credential.name}</TableCell><TableCell>{credential.provider}</TableCell><TableCell>{credential.zones?.map((zone) => zone.zone_name).join(", ") || t("common.none")}</TableCell>{props.canManage ? <TableCell className="flex gap-2"><IconButton onClick={() => props.onDetail(credential)}><EyeIcon /></IconButton><IconButton onClick={() => props.onEdit(credential)}><Edit3Icon /></IconButton><IconButton onClick={() => props.onDelete(credential)}><Trash2Icon /></IconButton></TableCell> : null}</TableRow>)}</TableBody></Table></DataState></CardContent>
    </Card>
  );
}

function ManagedRecordTable(props: { canManage: boolean; records: DNSManagedRecord[]; instances: DNSInstance[]; loading: boolean; error: string; onNew: () => void; onRefresh: () => void; onEvaluate: (record: DNSManagedRecord) => void; onDetail: (record: DNSManagedRecord) => void; onEdit: (record: DNSManagedRecord) => void; onDelete: (record: DNSManagedRecord) => void }) {
  const { t } = useI18n();
  return (
    <Card>
      <CardHeader><CardTitle>{t("dns.managedRecords")}</CardTitle><CardAction className="flex gap-2">{props.canManage ? <Button onClick={props.onNew} size="sm" type="button"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button> : null}<Button onClick={props.onRefresh} size="icon" type="button" variant="outline"><RefreshCwIcon /></Button></CardAction></CardHeader>
      <CardContent><DataState loading={props.loading} loadingFallback={<TableSkeleton columns={props.canManage ? 6 : 5} rows={4} />} error={props.error}><Table><TableHeader><TableRow><TableHead>{t("dns.record")}</TableHead><TableHead>{t("dns.type")}</TableHead><TableHead>{t("dns.activeInstance")}</TableHead><TableHead>{t("dns.values")}</TableHead><TableHead>{t("field.status")}</TableHead>{props.canManage ? <TableHead>{t("common.actions")}</TableHead> : null}</TableRow></TableHeader><TableBody>{props.records.map((record) => <TableRow key={record.id}><TableCell>{record.record_name}</TableCell><TableCell>{record.record_type}</TableCell><TableCell>{record.active_instance_id ? props.instances.find((instance) => instance.id === record.active_instance_id)?.name ?? record.active_instance_id : t("common.none")}</TableCell><TableCell className="max-w-[18rem] truncate">{record.last_applied_values.join(", ") || t("common.none")}</TableCell><TableCell><StatusBadge value={record.last_evaluation_status} /></TableCell>{props.canManage ? <TableCell className="flex gap-2"><IconButton onClick={() => props.onEvaluate(record)}><RefreshCwIcon /></IconButton><IconButton onClick={() => props.onDetail(record)}><EyeIcon /></IconButton><IconButton onClick={() => props.onEdit(record)}><Edit3Icon /></IconButton><IconButton onClick={() => props.onDelete(record)}><Trash2Icon /></IconButton></TableCell> : null}</TableRow>)}</TableBody></Table></DataState></CardContent>
    </Card>
  );
}

function InstanceTable(props: { canManage: boolean; instances: DNSInstance[]; records: DNSManagedRecord[]; loading: boolean; error: string; onNew: () => void; onRefresh: () => void; onDetail: (instance: DNSInstance) => void; onEdit: (instance: DNSInstance) => void; onDelete: (instance: DNSInstance) => void }) {
  const { t } = useI18n();
  return (
    <Card>
      <CardHeader><CardTitle>{t("dns.instances")}</CardTitle><CardAction className="flex gap-2">{props.canManage ? <Button disabled={props.records.length === 0} onClick={props.onNew} size="sm" type="button"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button> : null}<Button onClick={props.onRefresh} size="icon" type="button" variant="outline"><RefreshCwIcon /></Button></CardAction></CardHeader>
      <CardContent><DataState loading={props.loading} loadingFallback={<TableSkeleton columns={props.canManage ? 6 : 5} rows={4} />} error={props.error}><Table><TableHeader><TableRow><TableHead>{t("field.name")}</TableHead><TableHead>{t("dns.record")}</TableHead><TableHead>{t("dns.priority")}</TableHead><TableHead>{t("dns.action")}</TableHead><TableHead>{t("field.status")}</TableHead>{props.canManage ? <TableHead>{t("common.actions")}</TableHead> : null}</TableRow></TableHeader><TableBody>{props.instances.map((instance) => <TableRow key={instance.id}><TableCell>{instance.name}</TableCell><TableCell>{props.records.find((record) => record.id === instance.managed_record_id)?.record_name ?? instance.managed_record_id}</TableCell><TableCell>{instance.priority}</TableCell><TableCell>{String(instance.action.type ?? "")}</TableCell><TableCell><StatusBadge value={instance.enabled ? instance.last_status : "DISABLED"} /></TableCell>{props.canManage ? <TableCell className="flex gap-2"><IconButton onClick={() => props.onDetail(instance)}><EyeIcon /></IconButton><IconButton onClick={() => props.onEdit(instance)}><Edit3Icon /></IconButton><IconButton onClick={() => props.onDelete(instance)}><Trash2Icon /></IconButton></TableCell> : null}</TableRow>)}</TableBody></Table></DataState></CardContent>
    </Card>
  );
}

function NotificationChannelTable(props: { canManage: boolean; channels: NotificationChannel[]; loading: boolean; error: string; onNew: () => void; onRefresh: () => void; onDetail: (channel: NotificationChannel) => void; onEdit: (channel: NotificationChannel) => void; onDelete: (channel: NotificationChannel) => void }) {
  const { t } = useI18n();
  return (
    <Card>
      <CardHeader><CardTitle>{t("dns.notificationChannels")}</CardTitle><CardAction className="flex gap-2">{props.canManage ? <Button onClick={props.onNew} size="sm" type="button"><PlusIcon data-icon="inline-start" />{t("common.create")}</Button> : null}<Button onClick={props.onRefresh} size="icon" type="button" variant="outline"><RefreshCwIcon /></Button></CardAction></CardHeader>
      <CardContent><DataState loading={props.loading} loadingFallback={<TableSkeleton columns={props.canManage ? 4 : 3} rows={4} />} error={props.error}><Table><TableHeader><TableRow><TableHead>{t("field.name")}</TableHead><TableHead>{t("dns.channelType")}</TableHead><TableHead>{t("common.enabled")}</TableHead>{props.canManage ? <TableHead>{t("common.actions")}</TableHead> : null}</TableRow></TableHeader><TableBody>{props.channels.map((channel) => <TableRow key={channel.id}><TableCell>{channel.name}</TableCell><TableCell>{channel.channel_type}</TableCell><TableCell><StatusBadge value={channel.enabled ? "ENABLED" : "DISABLED"} /></TableCell>{props.canManage ? <TableCell className="flex gap-2"><IconButton onClick={() => props.onDetail(channel)}><EyeIcon /></IconButton><IconButton onClick={() => props.onEdit(channel)}><Edit3Icon /></IconButton><IconButton onClick={() => props.onDelete(channel)}><Trash2Icon /></IconButton></TableCell> : null}</TableRow>)}</TableBody></Table></DataState></CardContent>
    </Card>
  );
}

function DNSCredentialCreateDrawer({ open, onOpenChange, onSaved }: { open: boolean; onOpenChange: (open: boolean) => void; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    setSaving(true);
    try {
      await controlPost<DNSCredential>("/api/control/dns/credentials", { name: String(form.get("name") ?? ""), provider: "CLOUDFLARE", secret: String(form.get("secret") ?? "") });
      formElement.reset();
      toast.success(t("dns.credentialCreated"));
      await onSaved();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <DNSDrawer title={t("dns.createCredential")} open={open} onOpenChange={onOpenChange}><DNSCredentialForm saving={saving} onSubmit={submit} /></DNSDrawer>;
}

function DNSCredentialEditDrawer({ credential, onOpenChange, onSaved }: { credential?: DNSCredential; onOpenChange: (open: boolean) => void; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!credential) return;
    const form = new FormData(event.currentTarget);
    setSaving(true);
    try {
      await controlPatch<DNSCredential>(`/api/control/dns/credentials/${credential.id}`, { name: String(form.get("name") ?? ""), provider: "CLOUDFLARE", secret: String(form.get("secret") ?? "") });
      toast.success(t("common.saved"));
      await onSaved();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <DNSDrawer title={t("common.edit")} open={Boolean(credential)} onOpenChange={onOpenChange}><DNSCredentialForm credential={credential} saving={saving} onSubmit={submit} /></DNSDrawer>;
}

function DNSCredentialDetailDrawer({ credential, onOpenChange, onSaved }: { credential?: DNSCredential; onOpenChange: (open: boolean) => void; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  async function refreshZones() {
    if (!credential) return;
    try {
      await controlPost<DNSCredential>(`/api/control/dns/credentials/${credential.id}/zones/refresh`, {});
      toast.success(t("common.saved"));
      await onSaved();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }
  return (
    <DNSDrawer title={credential?.name ?? t("dns.credential")} open={Boolean(credential)} onOpenChange={onOpenChange}>
      <div className="grid gap-3 px-4 text-sm">
        <DetailRow label="Provider" value={credential?.provider ?? ""} />
        <Button onClick={refreshZones} type="button" variant="outline"><RefreshCwIcon data-icon="inline-start" />{t("dns.refreshZones")}</Button>
        {(credential?.zones ?? []).map((zone) => <DetailRow key={zone.id} label={zone.zone_name} value={`${zone.status} · ${shortDate(zone.last_synced_at, locale)}`} />)}
        {(credential?.zones?.length ?? 0) === 0 ? <p className="text-muted-foreground">{t("common.none")}</p> : null}
      </div>
    </DNSDrawer>
  );
}

function DNSManagedRecordCreateDrawer(props: DNSManagedRecordDrawerProps & { open: boolean }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    setSaving(true);
    try {
      await controlPost<DNSManagedRecord>("/api/control/dns/managed-records", dnsManagedRecordPayloadFromForm(formElement));
      formElement.reset();
      toast.success(t("dns.recordCreated"));
      await props.onSaved();
      props.onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <DNSDrawer title={t("dns.createRecord")} open={props.open} onOpenChange={props.onOpenChange}><DNSManagedRecordForm key="create" {...props} saving={saving} onSubmit={submit} /></DNSDrawer>;
}

function DNSManagedRecordEditDrawer(props: DNSManagedRecordDrawerProps & { record?: DNSManagedRecord }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!props.record) return;
    const formElement = event.currentTarget;
    setSaving(true);
    try {
      await controlPatch<DNSManagedRecord>(`/api/control/dns/managed-records/${props.record.id}`, dnsManagedRecordPayloadFromForm(formElement));
      toast.success(t("common.saved"));
      await props.onSaved();
      props.onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <DNSDrawer title={t("common.edit")} open={Boolean(props.record)} onOpenChange={props.onOpenChange}><DNSManagedRecordForm key={props.record?.id ?? "empty"} {...props} saving={saving} onSubmit={submit} /></DNSDrawer>;
}

function DNSManagedRecordDetailDrawer({ record, instances, onOpenChange }: { record?: DNSManagedRecord; instances: DNSInstance[]; onOpenChange: (open: boolean) => void }) {
  const { locale, t } = useI18n();
  const active = instances.find((instance) => instance.id === record?.active_instance_id);
  return (
    <DNSDrawer title={record?.record_name ?? t("dns.record")} open={Boolean(record)} onOpenChange={onOpenChange}>
      <div className="grid gap-3 px-4 text-sm">
        <DetailRow label={t("dns.zone")} value={record?.zone_name || ""} />
        <DetailRow label={t("dns.type")} value={record?.record_type ?? ""} />
        <DetailRow label={t("dns.activeInstance")} value={active?.name ?? record?.active_instance_id ?? t("common.none")} />
        <DetailRow label={t("dns.values")} value={record?.last_applied_values.join(", ") || t("common.none")} />
        <DetailRow label={t("field.status")} value={record?.last_evaluation_status ?? ""} />
        <DetailRow label={t("dns.diagnostics")} value={record?.last_diagnostics.map((item) => `${item.code}: ${item.message}`).join("\n") || t("common.none")} />
        <DetailRow label={t("dns.lastApplied")} value={shortDate(record?.last_applied_at, locale)} />
      </div>
    </DNSDrawer>
  );
}

function DNSInstanceCreateDrawer(props: DNSInstanceDrawerProps & { open: boolean }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    setSaving(true);
    try {
      await controlPost<DNSInstance>("/api/control/dns/instances", dnsInstancePayloadFromForm(formElement));
      formElement.reset();
      toast.success(t("dns.instanceCreated"));
      await props.onSaved();
      props.onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <DNSDrawer title={t("dns.createInstance")} open={props.open} onOpenChange={props.onOpenChange}><DNSInstanceForm key="create" {...props} saving={saving} onSubmit={submit} /></DNSDrawer>;
}

function DNSInstanceEditDrawer(props: DNSInstanceDrawerProps & { instance?: DNSInstance }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!props.instance) return;
    const formElement = event.currentTarget;
    setSaving(true);
    try {
      await controlPatch<DNSInstance>(`/api/control/dns/instances/${props.instance.id}`, dnsInstancePayloadFromForm(formElement));
      toast.success(t("common.saved"));
      await props.onSaved();
      props.onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <DNSDrawer title={t("common.edit")} open={Boolean(props.instance)} onOpenChange={props.onOpenChange}><DNSInstanceForm key={props.instance?.id ?? "empty"} {...props} saving={saving} onSubmit={submit} /></DNSDrawer>;
}

function DNSInstanceDetailDrawer({ instance, managedRecords, onOpenChange }: { instance?: DNSInstance; managedRecords: DNSManagedRecord[]; onOpenChange: (open: boolean) => void }) {
  const { locale, t } = useI18n();
  const record = managedRecords.find((candidate) => candidate.id === instance?.managed_record_id);
  return (
    <DNSDrawer title={instance?.name ?? t("dns.instance")} open={Boolean(instance)} onOpenChange={onOpenChange}>
      <div className="grid gap-3 px-4 text-sm">
        <DetailRow label={t("dns.record")} value={record?.record_name ?? instance?.managed_record_id ?? ""} />
        <DetailRow label={t("dns.priority")} value={String(instance?.priority ?? "")} />
        <DetailRow label={t("dns.values")} value={instance?.last_output_values.join(", ") || t("common.none")} />
        <DetailRow label={t("field.status")} value={instance?.last_status ?? ""} />
        <DetailRow label={t("dns.lastEvaluation")} value={shortDate(instance?.last_evaluated_at, locale)} />
        <DetailRow label={t("dns.condition")} value={jsonPretty(instance?.condition ?? {})} />
        <DetailRow label={t("dns.action")} value={jsonPretty(instance?.action ?? {})} />
      </div>
    </DNSDrawer>
  );
}

function NotificationChannelCreateDrawer({ open, onOpenChange, onSaved }: { open: boolean; onOpenChange: (open: boolean) => void; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    setSaving(true);
    try {
      await controlPost<NotificationChannel>("/api/control/notification-channels", notificationChannelPayloadFromForm(formElement));
      formElement.reset();
      toast.success(t("dns.channelCreated"));
      await onSaved();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <DNSDrawer title={t("dns.createChannel")} open={open} onOpenChange={onOpenChange}><NotificationChannelForm saving={saving} onSubmit={submit} /></DNSDrawer>;
}

function NotificationChannelEditDrawer({ channel, onOpenChange, onSaved }: { channel?: NotificationChannel; onOpenChange: (open: boolean) => void; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const [saving, setSaving] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!channel) return;
    const formElement = event.currentTarget;
    setSaving(true);
    try {
      await controlPatch<NotificationChannel>(`/api/control/notification-channels/${channel.id}`, notificationChannelPayloadFromForm(formElement));
      toast.success(t("common.saved"));
      await onSaved();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setSaving(false);
    }
  }
  return <DNSDrawer title={t("common.edit")} open={Boolean(channel)} onOpenChange={onOpenChange}><NotificationChannelForm key={channel?.id ?? "empty"} channel={channel} saving={saving} onSubmit={submit} /></DNSDrawer>;
}

function NotificationChannelDetailDrawer({ channel, onOpenChange }: { channel?: NotificationChannel; onOpenChange: (open: boolean) => void }) {
  const { t } = useI18n();
  return (
    <DNSDrawer title={channel?.name ?? t("dns.notificationChannel")} open={Boolean(channel)} onOpenChange={onOpenChange}>
      <div className="grid gap-3 px-4 text-sm">
        <DetailRow label={t("dns.channelType")} value={channel?.channel_type ?? ""} />
        <DetailRow label={t("common.enabled")} value={channel?.enabled ? t("common.enabled") : t("common.disabled")} />
        <DetailRow label={t("health.config")} value={jsonPretty(channel?.config ?? {})} />
      </div>
    </DNSDrawer>
  );
}

function DNSCredentialForm({ credential, saving, onSubmit }: { credential?: DNSCredential; saving: boolean; onSubmit: (event: FormEvent<HTMLFormElement>) => void }) {
  const { t } = useI18n();
  return (
    <form className="grid gap-4 px-4" onSubmit={onSubmit}>
      <Field><FieldLabel htmlFor="dns-credential-name">{t("field.name")}</FieldLabel><Input defaultValue={credential?.name ?? ""} id="dns-credential-name" name="name" required /></Field>
      <Field><FieldLabel htmlFor="dns-secret">Cloudflare token</FieldLabel><Input id="dns-secret" name="secret" required={!credential} type="password" /></Field>
      <Button disabled={saving} type="submit">{t("common.save")}</Button>
    </form>
  );
}

type DNSManagedRecordDrawerProps = {
  credentials: DNSCredential[];
  onOpenChange: (open: boolean) => void;
  onSaved: () => Promise<void>;
};

function DNSManagedRecordForm({ record, credentials, saving, onSubmit }: DNSManagedRecordDrawerProps & { record?: DNSManagedRecord; saving: boolean; onSubmit: (event: FormEvent<HTMLFormElement>) => void }) {
  const { t } = useI18n();
  const defaultCredentialID = record?.dns_credential_id ?? credentials[0]?.id ?? "";
  const [credentialID, setCredentialID] = useState(defaultCredentialID);
  const credential = credentials.find((candidate) => candidate.id === credentialID);
  const currentZoneID = credentialID === record?.dns_credential_id ? record?.credential_zone_id ?? "" : "";
  const zones = (credential?.zones ?? []).filter((zone) => dnsCredentialZoneWritable(zone.status) || zone.id === currentZoneID);
  const [zoneID, setZoneID] = useState(record?.credential_zone_id ?? zones[0]?.id ?? "");
  const zone = zones.find((candidate) => candidate.id === zoneID);
  const [recordType, setRecordType] = useState(record?.record_type ?? "A");
  const [proxied, setProxied] = useState(record?.proxied ?? false);
  useEffect(() => {
    const nextCredential = credentials.find((candidate) => candidate.id === credentialID);
    const nextCurrentZoneID = credentialID === record?.dns_credential_id ? record?.credential_zone_id ?? "" : "";
    const nextZones = (nextCredential?.zones ?? []).filter((candidate) => dnsCredentialZoneWritable(candidate.status) || candidate.id === nextCurrentZoneID);
    if (!nextZones.some((candidate) => candidate.id === zoneID)) setZoneID(nextZones[0]?.id ?? "");
  }, [credentialID, credentials, record?.credential_zone_id, record?.dns_credential_id, zoneID]);
  return (
    <form className="grid gap-4 px-4" onSubmit={onSubmit}>
      <SelectField label={t("field.dns_credential_id")} name="dns_credential_id" onChange={setCredentialID} options={credentials.map((item) => ({ value: item.id, label: item.name }))} value={credentialID} />
      <SelectField label={t("dns.zone")} name="credential_zone_id" onChange={setZoneID} options={zones.map((item) => ({ value: item.id, label: dnsCredentialZoneWritable(item.status) ? item.zone_name : `${item.zone_name} (${item.status})` }))} value={zoneID} />
      <Field><FieldLabel htmlFor="dns-record-host">{t("dns.record")}</FieldLabel><div className="flex"><Input className="rounded-r-none" defaultValue={record?.record_host === "@" ? "" : record?.record_host ?? ""} id="dns-record-host" name="record_host" placeholder="www" /><span className="flex h-9 items-center rounded-r-md border border-l-0 bg-muted px-3 text-sm text-muted-foreground">{zone?.zone_name ? `.${zone.zone_name}` : ""}</span></div></Field>
      <EnumSelect label={t("dns.type")} onValueChange={setRecordType} options={[{ value: "A", label: "A" }, { value: "AAAA", label: "AAAA" }, { value: "CNAME", label: "CNAME" }]} value={recordType} />
      <input name="record_type" type="hidden" value={recordType} />
      <Field><FieldLabel htmlFor="dns-ttl">TTL</FieldLabel><Input defaultValue={String(record?.ttl ?? 60)} id="dns-ttl" min="1" name="ttl" required type="number" /></Field>
      <Field orientation="horizontal"><Switch checked={proxied} onCheckedChange={setProxied} /> <FieldLabel>Cloudflare proxy</FieldLabel></Field>
      <input name="proxied" type="hidden" value={proxied ? "true" : "false"} />
      <Button disabled={saving || !credentialID || !zoneID} type="submit">{t("common.save")}</Button>
    </form>
  );
}

type DNSInstanceDrawerProps = {
  managedRecords: DNSManagedRecord[];
  instances: DNSInstance[];
  nodeGroups: ResourceOption[];
  channels: NotificationChannel[];
  onOpenChange: (open: boolean) => void;
  onSaved: () => Promise<void>;
};

function DNSInstanceForm({ instance, managedRecords, instances, nodeGroups, channels, saving, onSubmit }: DNSInstanceDrawerProps & { instance?: DNSInstance; saving: boolean; onSubmit: (event: FormEvent<HTMLFormElement>) => void }) {
  const { t } = useI18n();
  const [enabled, setEnabled] = useState(instance?.enabled ?? true);
  const [actionType, setActionType] = useState(String(instance?.action.type ?? "ROTATE_ONLINE_NODES"));
  return (
    <form className="grid gap-4 px-4" onSubmit={onSubmit}>
      <SelectField defaultValue={instance?.managed_record_id ?? managedRecords[0]?.id ?? ""} label={t("dns.record")} name="managed_record_id" options={managedRecords.map((record) => ({ value: record.id, label: `${record.record_name} ${record.record_type}` }))} />
      <Field><FieldLabel htmlFor="dns-instance-name">{t("field.name")}</FieldLabel><Input defaultValue={instance?.name ?? ""} id="dns-instance-name" name="name" required /></Field>
      <Field><FieldLabel htmlFor="dns-instance-priority">{t("dns.priority")}</FieldLabel><Input defaultValue={String(instance?.priority ?? 100)} id="dns-instance-priority" min="0" name="priority" required type="number" /></Field>
      <MultiSelectField defaultValues={instance?.node_group_ids ?? []} label={t("nodes.groups")} name="node_group_id" options={nodeGroups.map((group) => ({ value: group.value, label: group.label, disabled: group.disabled }))} required={false} />
      <Field><FieldLabel htmlFor="dns-answer-count">{t("dns.answerCount")}</FieldLabel><Input defaultValue={String(instance?.answer_count ?? -1)} id="dns-answer-count" name="answer_count" required type="number" /></Field>
      <DNSConditionBuilder value={instance?.condition ?? {}} />
      <EnumSelect label={t("dns.action")} onValueChange={setActionType} options={[{ value: "ROTATE_ONLINE_NODES", label: "ROTATE_ONLINE_NODES" }, { value: "SET_STATIC_ADDRESSES", label: "SET_STATIC_A/AAAA" }, { value: "SET_STATIC_CNAME", label: "SET_STATIC_CNAME" }, { value: "USE_INSTANCE_OUTPUT", label: "USE_INSTANCE_OUTPUT" }]} value={actionType} />
      <input name="action_type" type="hidden" value={actionType} />
      {actionType === "SET_STATIC_ADDRESSES" ? <Field><FieldLabel htmlFor="dns-action-values">{t("dns.values")}</FieldLabel><Input defaultValue={Array.isArray(instance?.action.values) ? instance?.action.values.join(", ") : ""} id="dns-action-values" name="action_values" /></Field> : null}
      {actionType === "SET_STATIC_CNAME" ? <Field><FieldLabel htmlFor="dns-action-cname">CNAME</FieldLabel><Input defaultValue={String(instance?.action.value ?? "")} id="dns-action-cname" name="action_value" /></Field> : null}
      {actionType === "USE_INSTANCE_OUTPUT" ? <SelectField defaultValue={String(instance?.action.instance_id ?? "")} label={t("dns.instance")} name="action_instance_id" options={instances.filter((candidate) => candidate.id !== instance?.id).map((candidate) => ({ value: candidate.id, label: candidate.name }))} /> : null}
      <MultiSelectField defaultValues={instance?.notification_channel_ids ?? []} label={t("dns.notificationChannels")} name="notification_channel_id" options={channels.map((channel) => ({ value: channel.id, label: channel.name }))} required={false} />
      <Field orientation="horizontal"><Switch checked={enabled} onCheckedChange={setEnabled} /> <FieldLabel>{t("common.enabled")}</FieldLabel></Field>
      <input name="enabled" type="hidden" value={enabled ? "true" : "false"} />
      <Button disabled={saving || managedRecords.length === 0} type="submit">{t("common.save")}</Button>
    </form>
  );
}

export type DNSConditionNode = DNSConditionGroup | DNSConditionLeaf | DNSConditionRaw;
export type DNSConditionGroup = { op: "AND" | "OR"; children: DNSConditionNode[]; preserveEmpty?: boolean; rawPayload?: Record<string, unknown> };
export type DNSConditionLeaf = { metric: string; comparator: string; value: string };
export type DNSConditionRaw = { raw: unknown };

const dnsConditionMetrics = [
  { value: "offline_node_count", labelKey: "dns.conditionOfflineCount" },
  { value: "online_node_count", labelKey: "dns.conditionOnlineCount" },
  { value: "offline_node_percent", labelKey: "dns.conditionOfflinePercent" },
  { value: "online_node_percent", labelKey: "dns.conditionOnlinePercent" },
];

const dnsConditionComparators = [">", "<", ">=", "<=", "="];

function DNSConditionBuilder({ value }: { value: Record<string, unknown> }) {
  const { t } = useI18n();
  const [condition, setCondition] = useState<DNSConditionGroup>(() => dnsConditionFromPayload(value));
  return (
    <Field>
      <FieldLabel>{t("dns.condition")}</FieldLabel>
      <div className="grid gap-2 rounded-md border p-3" data-testid="dns-condition-builder">
        <DNSConditionGroupEditor node={condition} onChange={setCondition} />
        {dnsConditionShowsAlwaysMatch(condition) ? <p className="text-sm text-muted-foreground">{t("dns.conditionAlways")}</p> : null}
      </div>
      <input data-testid="dns-condition-payload" name="condition_payload" type="hidden" value={JSON.stringify(dnsConditionToPayload(condition))} />
    </Field>
  );
}

function DNSConditionGroupEditor({ node, onChange, onRemove, depth = 0 }: { node: DNSConditionGroup; onChange: (node: DNSConditionGroup) => void; onRemove?: () => void; depth?: number }) {
  const { t } = useI18n();
  function updateChild(index: number, child: DNSConditionNode) {
    onChange({ ...node, children: node.children.map((candidate, candidateIndex) => candidateIndex === index ? child : candidate) });
  }
  function removeChild(index: number) {
    const children = node.children.filter((_, candidateIndex) => candidateIndex !== index);
    onChange({ ...node, children, preserveEmpty: depth === 0 && children.length === 0 ? false : node.preserveEmpty });
  }
  return (
    <div className={depth > 0 ? "grid min-w-0 gap-2 rounded-md border p-3" : "grid min-w-0 gap-2"}>
      <div className="flex flex-wrap items-center gap-2">
        <select className="h-8 min-w-0 rounded-md border bg-background px-2.5 text-sm" value={node.op} onChange={(event) => onChange({ ...node, op: event.currentTarget.value === "OR" ? "OR" : "AND" })}>
          <option value="AND">AND</option>
          <option value="OR">OR</option>
        </select>
        <Button data-testid="dns-condition-add-condition" onClick={() => onChange({ ...node, rawPayload: undefined, children: [...node.children, defaultDNSConditionLeaf()] })} type="button" variant="outline"><PlusIcon data-icon="inline-start" />{t("dns.addCondition")}</Button>
        <Button data-testid="dns-condition-add-group" onClick={() => onChange({ ...node, rawPayload: undefined, children: [...node.children, defaultDNSConditionGroup()] })} type="button" variant="outline"><PlusIcon data-icon="inline-start" />{t("dns.addGroup")}</Button>
        {onRemove ? <Button onClick={onRemove} size="icon" type="button" variant="outline"><Trash2Icon /></Button> : null}
      </div>
      {node.rawPayload ? <DNSConditionRawEditor node={{ raw: node.rawPayload }} onRemove={() => onChange({ ...node, rawPayload: undefined })} /> : null}
      <div className="grid gap-2">
        {node.children.map((child, index) => isDNSConditionGroup(child)
          ? <DNSConditionGroupEditor key={index} node={child} onChange={(next) => updateChild(index, next)} onRemove={() => removeChild(index)} depth={depth + 1} />
          : isDNSConditionRaw(child)
            ? <DNSConditionRawEditor key={index} node={child} onRemove={() => removeChild(index)} />
            : <DNSConditionLeafEditor key={index} node={child} onChange={(next) => updateChild(index, next)} onRemove={() => removeChild(index)} />)}
      </div>
    </div>
  );
}

function DNSConditionLeafEditor({ node, onChange, onRemove }: { node: DNSConditionLeaf; onChange: (node: DNSConditionLeaf) => void; onRemove: () => void }) {
  const { t } = useI18n();
  return (
    <div className="grid min-w-0 gap-2 rounded-md border p-3 sm:grid-cols-[minmax(0,1fr)_4.5rem_5rem_2rem] sm:items-center" data-testid="dns-condition-leaf">
      <select className="h-8 w-full min-w-0 rounded-md border bg-background px-2.5 text-sm" value={node.metric} onChange={(event) => onChange({ ...node, metric: event.currentTarget.value })}>
        {dnsConditionMetrics.map((metric) => <option key={metric.value} value={metric.value}>{t(metric.labelKey)}</option>)}
      </select>
      <select className="h-8 w-full min-w-0 rounded-md border bg-background px-2.5 text-sm" value={node.comparator} onChange={(event) => onChange({ ...node, comparator: event.currentTarget.value })}>
        {dnsConditionComparators.map((comparator) => <option key={comparator} value={comparator}>{comparator}</option>)}
      </select>
      <Input className="h-8" min="0" onChange={(event) => onChange({ ...node, value: event.currentTarget.value })} step="any" type="number" value={node.value} />
      <Button onClick={onRemove} size="icon" type="button" variant="outline"><Trash2Icon /></Button>
    </div>
  );
}

function DNSConditionRawEditor({ node, onRemove }: { node: DNSConditionRaw; onRemove: () => void }) {
  return (
    <div className="grid gap-2 rounded-md border p-3">
      <Textarea readOnly rows={4} value={jsonPrettyRaw(node.raw)} />
      <Button onClick={onRemove} size="icon-sm" type="button" variant="outline"><Trash2Icon /></Button>
    </div>
  );
}

function NotificationChannelForm({ channel, saving, onSubmit }: { channel?: NotificationChannel; saving: boolean; onSubmit: (event: FormEvent<HTMLFormElement>) => void }) {
  const { t } = useI18n();
  const [enabled, setEnabled] = useState(channel?.enabled ?? true);
  const [channelType, setChannelType] = useState(channel?.channel_type ?? "WEBHOOK");
  const [configText, setConfigText] = useState(jsonPretty(channel?.config ?? defaultNotificationConfig(channel?.channel_type ?? "WEBHOOK")));
  function updateChannelType(nextType: string) {
    setChannelType(nextType);
    setConfigText(jsonPretty(channel?.channel_type === nextType ? channel.config : defaultNotificationConfig(nextType)));
  }
  return (
    <form className="grid gap-4 px-4" onSubmit={onSubmit}>
      <Field><FieldLabel htmlFor="notification-name">{t("field.name")}</FieldLabel><Input defaultValue={channel?.name ?? ""} id="notification-name" name="name" required /></Field>
      <EnumSelect label={t("dns.channelType")} onValueChange={updateChannelType} options={[{ value: "WEBHOOK", label: "WEBHOOK" }, { value: "EMAIL", label: "EMAIL" }]} value={channelType} />
      <input name="channel_type" type="hidden" value={channelType} />
      <Field><FieldLabel htmlFor="notification-config">{t("health.config")}</FieldLabel><Textarea id="notification-config" name="config_json" onChange={(event) => setConfigText(event.currentTarget.value)} rows={7} value={configText} /></Field>
      <Field><FieldLabel htmlFor="notification-secret">{channelType === "EMAIL" ? "SMTP password" : "Secret"}</FieldLabel><Input id="notification-secret" name="secret" required={!channel && channelType === "EMAIL"} type="password" /></Field>
      <Field orientation="horizontal"><Switch checked={enabled} onCheckedChange={setEnabled} /> <FieldLabel>{t("common.enabled")}</FieldLabel></Field>
      <input name="enabled" type="hidden" value={enabled ? "true" : "false"} />
      <Button disabled={saving} type="submit">{t("common.save")}</Button>
    </form>
  );
}

function IconButton({ children, onClick }: { children: ReactNode; onClick: () => void }) {
  return <Button onClick={onClick} size="icon-sm" type="button" variant="outline">{children}</Button>;
}

function DNSDrawer({ title, open, onOpenChange, children }: { title: string; open: boolean; onOpenChange: (open: boolean) => void; children: ReactNode }) {
  return <Sheet open={open} onOpenChange={onOpenChange}><SheetContent className="overflow-y-auto sm:max-w-lg"><SheetHeader><SheetTitle>{title}</SheetTitle></SheetHeader>{children}</SheetContent></Sheet>;
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return <div className="grid gap-1"><span className="text-muted-foreground">{label}</span><span className="whitespace-pre-wrap break-words">{value}</span></div>;
}

function SelectField({ label, name, options, defaultValue, value, onChange }: { label: string; name: string; options: ResourceOption[]; defaultValue?: string; value?: string; onChange?: (value: string) => void }) {
  const { t } = useI18n();
  return (
    <Field>
      <FieldLabel htmlFor={name}>{label}</FieldLabel>
      <select className="h-9 rounded-md border bg-background px-3 text-sm" defaultValue={value === undefined ? defaultValue : undefined} id={name} name={name} onChange={(event) => onChange?.(event.currentTarget.value)} required value={value}>
        <option value="">{t("resource.selectResource")}</option>
        {options.map((option) => <option disabled={option.disabled} key={option.value} value={option.value}>{option.label}</option>)}
      </select>
    </Field>
  );
}

function dnsDeleteLabel(request: DeleteRequest) {
  if (request.kind === "managed_record") return request.item.record_name;
  return request.item.name;
}

function dnsManagedRecordPayloadFromForm(formElement: HTMLFormElement) {
  const form = new FormData(formElement);
  return { dns_credential_id: String(form.get("dns_credential_id") ?? ""), credential_zone_id: String(form.get("credential_zone_id") ?? ""), record_host: String(form.get("record_host") ?? ""), record_type: String(form.get("record_type") ?? "A"), ttl: Number(form.get("ttl") ?? 60), proxied: String(form.get("proxied") ?? "false") === "true" };
}

function dnsInstancePayloadFromForm(formElement: HTMLFormElement) {
  const form = new FormData(formElement);
  const actionType = String(form.get("action_type") ?? "ROTATE_ONLINE_NODES");
  const action: Record<string, unknown> = { type: actionType };
  if (actionType === "SET_STATIC_ADDRESSES") action.values = String(form.get("action_values") ?? "").split(/\s*,\s*/).filter(Boolean);
  if (actionType === "SET_STATIC_CNAME") action.value = String(form.get("action_value") ?? "");
  if (actionType === "USE_INSTANCE_OUTPUT") action.instance_id = String(form.get("action_instance_id") ?? "");
  return { managed_record_id: String(form.get("managed_record_id") ?? ""), name: String(form.get("name") ?? ""), priority: Number(form.get("priority") ?? 100), enabled: String(form.get("enabled") ?? "true") === "true", node_group_ids: form.getAll("node_group_id").map(String).filter(Boolean), answer_count: Number(form.get("answer_count") ?? -1), condition: parseJSONFormField(form, "condition_payload", {}), action, notification_channel_ids: form.getAll("notification_channel_id").map(String).filter(Boolean) };
}

function notificationChannelPayloadFromForm(formElement: HTMLFormElement) {
  const form = new FormData(formElement);
  return { name: String(form.get("name") ?? ""), channel_type: String(form.get("channel_type") ?? "WEBHOOK"), config: parseJSONFormField(form, "config_json", {}), secret: String(form.get("secret") ?? ""), enabled: String(form.get("enabled") ?? "true") === "true" };
}

function parseJSONFormField(form: FormData, key: string, fallback: Record<string, unknown>) {
  const value = String(form.get(key) ?? "").trim();
  if (!value) return fallback;
  const parsed = JSON.parse(value) as unknown;
  return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed as Record<string, unknown> : fallback;
}

function jsonPretty(value: unknown) {
  return JSON.stringify(value ?? {}, null, 2);
}

function jsonPrettyRaw(value: unknown) {
  return JSON.stringify(value, null, 2) ?? String(value);
}

function defaultNotificationConfig(channelType: string) {
  if (channelType === "EMAIL") return { smtp_host: "", smtp_port: 587, username: "", from: "", to: [], subject: "Prism DNS policy notification" };
  return { url: "", method: "POST", headers: {} };
}

function dnsCredentialZoneWritable(status: string) {
  const normalized = status.trim().toUpperCase();
  return normalized === "ACTIVE" || normalized === "PENDING";
}

function defaultDNSConditionGroup(preserveEmpty = false): DNSConditionGroup {
  return { op: "AND", children: [], preserveEmpty };
}

function defaultDNSConditionLeaf(): DNSConditionLeaf {
  return { metric: "online_node_count", comparator: ">=", value: "1" };
}

export function dnsConditionFromPayload(value: Record<string, unknown>): DNSConditionGroup {
  const node = dnsConditionNodeFromPayload(value);
  if (isDNSConditionGroup(node)) return node;
  if (isDNSConditionRaw(node) && isRecord(node.raw)) return { op: "AND", children: [], rawPayload: node.raw };
  if (node) return { op: "AND", children: [node] };
  return defaultDNSConditionGroup();
}

function dnsConditionNodeFromPayload(value: unknown, preserveMalformed = false): DNSConditionNode | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return preserveMalformed ? { raw: value } : null;
  const raw = value as Record<string, unknown>;
  if (Object.keys(raw).length === 0) return preserveMalformed ? { raw: { ...raw } } : null;
  const op = normalizeDNSConditionOperator(raw.op);
  if (op === "AND" || op === "OR") {
    const children = Array.isArray(raw.children) ? raw.children.map((child) => dnsConditionNodeFromPayload(child, true)).filter((child): child is DNSConditionNode => Boolean(child)) : [];
    return { op, children, preserveEmpty: true };
  }
  const metric = normalizeDNSConditionMetric(raw.metric);
  const comparator = normalizeDNSConditionComparator(raw.comparator);
  const numeric = typeof raw.value === "number" ? raw.value : Number.NaN;
  if (dnsConditionMetrics.some((candidate) => candidate.value === metric) && dnsConditionComparators.includes(comparator) && Number.isFinite(numeric)) {
    return { metric, comparator, value: String(numeric) };
  }
  return { raw: { ...raw } };
}

export function dnsConditionToPayload(node: DNSConditionGroup): Record<string, unknown> {
  if (node.rawPayload && node.children.length === 0) return node.rawPayload;
  const children = node.children.map(dnsConditionNodeToPayload).filter((child) => child !== dnsConditionOmit);
  if (children.length === 0) return node.preserveEmpty ? { op: node.op, children: [] } : {};
  return { op: node.op, children };
}

const dnsConditionOmit = Symbol("dns-condition-omit");

function dnsConditionNodeToPayload(node: DNSConditionNode): unknown | typeof dnsConditionOmit {
  if (isDNSConditionRaw(node)) return node.raw;
  if (!isDNSConditionGroup(node)) {
    const numeric = node.value.trim() === "" ? Number.NaN : Number(node.value);
    return { metric: node.metric, comparator: node.comparator, value: Number.isFinite(numeric) ? numeric : node.value };
  }
  const children = node.children.map(dnsConditionNodeToPayload).filter((child) => child !== dnsConditionOmit);
  if (children.length === 0) return node.preserveEmpty ? { op: node.op, children: [] } : dnsConditionOmit;
  return { op: node.op, children };
}

export function dnsConditionShowsAlwaysMatch(node: DNSConditionGroup) {
  return node.children.length === 0 && !node.rawPayload && !node.preserveEmpty;
}

function isDNSConditionGroup(node: DNSConditionNode | null): node is DNSConditionGroup {
  return node !== null && "children" in node;
}

function isDNSConditionRaw(node: DNSConditionNode | null): node is DNSConditionRaw {
  return node !== null && "raw" in node;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function normalizeDNSConditionOperator(value: unknown) {
  return String(value ?? "").trim().toUpperCase();
}

function normalizeDNSConditionMetric(value: unknown) {
  return String(value ?? "").trim().toLowerCase();
}

function normalizeDNSConditionComparator(value: unknown) {
  return String(value ?? "").trim();
}
