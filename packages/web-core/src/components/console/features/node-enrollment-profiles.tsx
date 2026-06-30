"use client";

import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@noxaaa/prism-oss-web-core/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@noxaaa/prism-oss-web-core/ui/dialog";
import { Field, FieldDescription, FieldLabel, FieldSet, FieldLegend } from "@noxaaa/prism-oss-web-core/ui/field";
import { Input } from "@noxaaa/prism-oss-web-core/ui/input";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@noxaaa/prism-oss-web-core/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@noxaaa/prism-oss-web-core/ui/table";
import { controlDelete, controlGet, controlPatch, controlPost, shortDate } from "@noxaaa/prism-oss-web-core/console/control-api";
import { localizeControlError, localizeEnum, useI18n } from "@noxaaa/prism-oss-web-core/console/i18n";
import { ResourceMultiSelect } from "@noxaaa/prism-oss-web-core/console/resource-select";
import { EnumSelect, FieldRequirementBadge, StatusBadge, TextAreaField, TextField } from "@noxaaa/prism-oss-web-core/console/shared";
import type { NodeEnrollmentEvent, NodeEnrollmentProfile, NodeGroup } from "@noxaaa/prism-oss-web-core/console/types";

const protocolOptions = [
  { value: "TCP", label: "TCP" },
  { value: "UDP", label: "UDP" },
  { value: "TCP_UDP", label: "TCP + UDP" },
];

const dataplaneModeOptions = [
  { value: "NATIVE", labelKey: "nodes.dataplaneNative" },
  { value: "AUTO", labelKey: "nodes.dataplaneAuto" },
  { value: "HAPROXY", labelKey: "nodes.dataplaneHAProxy" },
  { value: "NFTABLES", labelKey: "nodes.dataplaneNFTables" },
];

type EditableIP = {
  address: string;
  display_name: string;
};

function nodePortRangesForSelection(protocol: string, startPort: number, endPort: number) {
  const protocols = protocol === "TCP_UDP" ? ["TCP", "UDP"] : [protocol];
  return protocols.map((rangeProtocol) => ({
    protocol: rangeProtocol,
    start_port: startPort,
    end_port: endPort,
  }));
}

export function NodeEnrollmentCreateDrawer({ groups, onCreated, onOpenChange, open }: { groups: NodeGroup[]; onCreated: (profile: NodeEnrollmentProfile) => Promise<void>; onOpenChange: (open: boolean) => void; open: boolean }) {
  const { t } = useI18n();
  async function handleCreated(profile: NodeEnrollmentProfile) {
    await onCreated(profile);
    onOpenChange(false);
  }
  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("nodes.createEnrollmentProfile")}</SheetTitle>
          <SheetDescription>{t("nodes.createEnrollmentProfileDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          <NodeEnrollmentMutationForm groups={groups} onSaved={handleCreated} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

export function NodeEnrollmentEditDrawer({ groups, onOpenChange, onUpdated, profile }: { groups: NodeGroup[]; onOpenChange: (open: boolean) => void; onUpdated: () => Promise<void>; profile: NodeEnrollmentProfile | null }) {
  const { t } = useI18n();
  async function handleUpdated() {
    await onUpdated();
    onOpenChange(false);
  }
  return (
    <Sheet onOpenChange={onOpenChange} open={Boolean(profile)}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("nodes.editEnrollmentProfile")}</SheetTitle>
          <SheetDescription>{t("nodes.editEnrollmentProfileDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          {profile ? <NodeEnrollmentMutationForm groups={groups} onSaved={async () => { await handleUpdated(); }} profile={profile} /> : null}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function NodeEnrollmentMutationForm({ groups, onSaved, profile }: { groups: NodeGroup[]; onSaved: (profile: NodeEnrollmentProfile) => Promise<void>; profile?: NodeEnrollmentProfile }) {
  const { locale, t } = useI18n();
  const initialProtocol = profile?.port_ranges?.length === 2 ? "TCP_UDP" : (profile?.port_ranges?.[0]?.protocol ?? "TCP");
  const initialPortRange = profile?.port_ranges?.[0];
  const [groupIDs, setGroupIDs] = useState<string[]>(profile?.group_ids ?? []);
  const [protocol, setProtocol] = useState(initialProtocol);
  const [dataplaneMode, setDataplaneMode] = useState(profile?.dataplane_mode ?? "AUTO");
  const [listenIPs, setListenIPs] = useState<EditableIP[]>(
    profile?.listen_ips?.length ? profile.listen_ips.map((item) => ({ address: item.listen_ip, display_name: item.display_name })) : [{ address: "0.0.0.0", display_name: "default" }],
  );
  const [sendIPs, setSendIPs] = useState<EditableIP[]>(
    profile?.send_ips?.length ? profile.send_ips.map((item) => ({ address: item.send_ip, display_name: item.display_name })) : [],
  );
  const [neverExpires, setNeverExpires] = useState(profile ? !profile.expires_at : false);
  const groupOptions = groups.map((group) => ({ value: group.id, label: group.name }));
  const localizedProtocolOptions = protocolOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));
  const localizedDataplaneOptions = dataplaneModeOptions.map((option) => ({ value: option.value, label: t(option.labelKey) }));
  const ttlHours = profile?.expires_at ? String(Math.max(0, Math.ceil((Date.parse(profile.expires_at) - Date.now()) / 3600000))) : "720";

  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const normalizedListenIPs = listenIPs
      .map((item) => ({ listen_ip: item.address.trim(), display_name: item.display_name.trim() }))
      .filter((item) => item.listen_ip !== "");
    if (normalizedListenIPs.length === 0) {
      normalizedListenIPs.push({ listen_ip: "0.0.0.0", display_name: "default" });
    }
    const normalizedSendIPs = sendIPs
      .map((item) => ({ send_ip: item.address.trim(), display_name: item.display_name.trim() }))
      .filter((item) => item.send_ip !== "");
    const portRanges = profile && protocol === initialProtocol && String(form.get("start_port") || "") === String(initialPortRange?.start_port ?? "") && String(form.get("end_port") || "") === String(initialPortRange?.end_port ?? "")
      ? profile.port_ranges
      : nodePortRangesForSelection(protocol, Number(form.get("start_port") || 10000), Number(form.get("end_port") || 20000));
    const payload: Record<string, unknown> = {
      name: form.get("name"),
      description: form.get("description"),
      enabled: form.get("enabled") === "on",
      max_uses: Number(form.get("max_uses") || 0),
      node_name_template: form.get("node_name_template"),
      group_ids: groupIDs,
      listen_ips: normalizedListenIPs,
      send_ips: normalizedSendIPs,
      port_ranges: portRanges,
      max_rule_ports: Number(form.get("max_rule_ports") || profile?.max_rule_ports || 256),
      dns_publish_addresses: profile?.dns_publish_addresses ?? [],
      dataplane_mode: dataplaneMode,
      dataplane_conflict_policy: "FAIL_FAST",
      auto_update_enabled: form.get("auto_update_enabled") === "on",
      allowed_cidrs: String(form.get("allowed_cidrs") || "").split(/\s*,\s*|\n/).map((value) => value.trim()).filter(Boolean),
    };
    applyNodeEnrollmentExpiryPayload(payload, {
      editing: Boolean(profile),
      initialExpiresAt: profile?.expires_at ?? "",
      initialTTLHours: ttlHours,
      neverExpires,
      submittedTTLHours: String(form.get("ttl_hours") || ""),
    });
    try {
      const result = profile
        ? await controlPatch<NodeEnrollmentProfile>(`/api/control/node-enrollment-profiles/${profile.id}`, payload)
        : await controlPost<NodeEnrollmentProfile>("/api/control/node-enrollment-profiles", payload);
      toast.success(profile ? t("nodes.enrollmentProfileUpdated") : t("nodes.enrollmentProfileCreated"));
      await onSaved(result);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={saveProfile}>
      <TextField defaultValue={profile?.name ?? ""} label={t("field.name")} name="name" placeholder="aws-prod-edge" />
      <TextAreaField defaultValue={profile?.description ?? ""} label={t("field.description")} name="description" placeholder="" />
      <ResourceMultiSelect label={t("nodes.nodeGroups")} onValueChange={setGroupIDs} options={groupOptions} values={groupIDs} />
      <TextField defaultValue={profile?.node_name_template ?? "{{hostname}}"} label={t("nodes.nodeNameTemplate")} name="node_name_template" placeholder="{{hostname}}" />
      <div className="grid gap-3 md:grid-cols-2">
        <TextField defaultValue={ttlHours} label={t("nodes.ttlHours")} name="ttl_hours" placeholder="720" required={!neverExpires} type="number" />
        <TextField defaultValue={String(profile?.max_uses ?? 0)} label={t("nodes.maxUses")} name="max_uses" placeholder="0" required={false} type="number" />
      </div>
      <label className="flex items-center gap-2 text-sm">
        <input checked={neverExpires} onChange={(event) => setNeverExpires(event.currentTarget.checked)} type="checkbox" />
        {t("nodes.neverExpireEnrollment")}
      </label>
      <EnumSelect label={t("nodes.dataplaneMode")} onValueChange={setDataplaneMode} options={localizedDataplaneOptions} value={dataplaneMode} />
      <IPListEditor description={t("nodes.listenIPsDescription")} items={listenIPs} label={t("rules.listenIP")} labelPlaceholder="default" legend={t("nodes.listenIPs")} onAdd={() => setListenIPs((current) => [...current, { address: "", display_name: "" }])} onRemove={(index) => setListenIPs((current) => current.filter((_, itemIndex) => itemIndex !== index))} onUpdate={(index, field, value) => setListenIPs((current) => current.map((item, itemIndex) => (itemIndex === index ? { ...item, [field]: value } : item)))} required />
      <IPListEditor description={t("nodes.sendIPsDescription")} items={sendIPs} label={t("nodes.sendIP")} labelPlaceholder="primary" legend={t("nodes.sendIPs")} onAdd={() => setSendIPs((current) => [...current, { address: "", display_name: "" }])} onRemove={(index) => setSendIPs((current) => current.filter((_, itemIndex) => itemIndex !== index))} onUpdate={(index, field, value) => setSendIPs((current) => current.map((item, itemIndex) => (itemIndex === index ? { ...item, [field]: value } : item)))} required={false} />
      <EnumSelect label={t("rules.protocol")} onValueChange={setProtocol} options={localizedProtocolOptions} value={protocol} />
      <div className="grid gap-3 md:grid-cols-3">
        <TextField defaultValue={String(initialPortRange?.start_port ?? 10000)} label={t("nodes.startPort")} name="start_port" placeholder="10000" type="number" />
        <TextField defaultValue={String(initialPortRange?.end_port ?? 20000)} label={t("nodes.endPort")} name="end_port" placeholder="20000" type="number" />
        <TextField defaultValue={String(profile?.max_rule_ports ?? 256)} label={t("nodes.maxRulePorts")} name="max_rule_ports" placeholder="256" type="number" />
      </div>
      <TextAreaField defaultValue={profile?.allowed_cidrs?.join("\n") ?? ""} label={t("nodes.allowedCIDRs")} name="allowed_cidrs" placeholder="203.0.113.0/24" />
      <label className="flex items-center gap-2 text-sm">
        <input defaultChecked={profile?.enabled ?? true} name="enabled" type="checkbox" />
        {t("common.enabled")}
      </label>
      <label className="flex items-center gap-2 text-sm">
        <input defaultChecked={profile?.auto_update_enabled ?? true} name="auto_update_enabled" type="checkbox" />
        {t("nodes.agentAutoUpdate")}
      </label>
      <Button disabled={groupIDs.length === 0} type="submit">
        {profile ? <PencilIcon data-icon="inline-start" /> : <PlusIcon data-icon="inline-start" />}
        {profile ? t("common.save") : t("common.create")}
      </Button>
    </form>
  );
}

export function applyNodeEnrollmentExpiryPayload(payload: Record<string, unknown>, options: { editing: boolean; initialExpiresAt: string; initialTTLHours: string; neverExpires: boolean; submittedTTLHours: string }) {
  if (options.neverExpires) {
    if (options.editing) {
      payload.expires_at = "";
    }
    return;
  }
  if (options.editing && options.initialExpiresAt && options.submittedTTLHours === options.initialTTLHours) {
    payload.expires_at = options.initialExpiresAt;
    return;
  }
  payload.ttl_hours = Number(options.submittedTTLHours || 720);
}

export function NodeEnrollmentDetailDrawer({ onOpenChange, profile }: { onOpenChange: (open: boolean) => void; profile: NodeEnrollmentProfile | null }) {
  const { locale, t } = useI18n();
  const [events, setEvents] = useState<NodeEnrollmentEvent[]>([]);

  useEffect(() => {
    if (!profile) {
      setEvents([]);
      return undefined;
    }
    let cancelled = false;
    controlGet<NodeEnrollmentEvent[]>(`/api/control/node-enrollment-profiles/${profile.id}/events`)
      .then((result) => {
        if (!cancelled) setEvents(result);
      })
      .catch(() => {
        if (!cancelled) setEvents([]);
      });
    return () => {
      cancelled = true;
    };
  }, [profile]);

  return (
    <Sheet onOpenChange={onOpenChange} open={Boolean(profile)}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{profile?.name ?? t("nodes.enrollmentProfile")}</SheetTitle>
          <SheetDescription>{t("nodes.enrollmentEventsDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          {profile ? (
            <div className="mb-4 grid gap-2 rounded-md border p-3 text-sm">
              <div className="flex justify-between gap-4"><span className="text-muted-foreground">{t("nodes.expiresAt")}</span><span>{profile.expires_at ? shortDate(profile.expires_at, locale) : t("nodes.neverExpires")}</span></div>
              <div className="flex justify-between gap-4"><span className="text-muted-foreground">{t("nodes.listenIPs")}</span><span className="text-right">{profile.listen_ips.map((item) => item.listen_ip).join(", ")}</span></div>
              <div className="flex justify-between gap-4"><span className="text-muted-foreground">{t("nodes.sendIPs")}</span><span className="text-right">{profile.send_ips?.length ? profile.send_ips.map((item) => item.send_ip).join(", ") : t("rules.defaultSendIP")}</span></div>
            </div>
          ) : null}
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("overview.status")}</TableHead>
                <TableHead>{t("nodes.hostname")}</TableHead>
                <TableHead>IP</TableHead>
                <TableHead>{t("common.createdAt")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {events.map((event) => (
                <TableRow key={event.id}>
                  <TableCell><StatusBadge value={event.reason_code || event.status} /></TableCell>
                  <TableCell>{event.hostname || t("common.unknown")}</TableCell>
                  <TableCell>{event.remote_ip || t("common.unknown")}</TableCell>
                  <TableCell>{shortDate(event.created_at, locale)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function IPListEditor({
  description,
  items,
  label,
  labelPlaceholder,
  legend,
  onAdd,
  onRemove,
  onUpdate,
  required,
}: {
  description: string;
  items: EditableIP[];
  label: string;
  labelPlaceholder: string;
  legend: string;
  onAdd: () => void;
  onRemove: (index: number) => void;
  onUpdate: (index: number, field: keyof EditableIP, value: string) => void;
  required: boolean;
}) {
  const { t } = useI18n();
  return (
    <FieldSet>
      <FieldLegend>{legend}</FieldLegend>
      <FieldDescription>{description}</FieldDescription>
      <div className="flex flex-col gap-3">
        {items.map((item, index) => (
          <div className="grid gap-3 md:grid-cols-[1fr_1fr_auto]" key={index}>
            <Field>
              <FieldLabel>{label}<FieldRequirementBadge required={required} /></FieldLabel>
              <Input onChange={(event) => onUpdate(index, "address", event.target.value)} placeholder="0.0.0.0" required={required} value={item.address} />
            </Field>
            <Field>
              <FieldLabel>{required ? t("nodes.listenIPLabel") : t("nodes.sendIPLabel")}<FieldRequirementBadge required={false} /></FieldLabel>
              <Input onChange={(event) => onUpdate(index, "display_name", event.target.value)} placeholder={labelPlaceholder} value={item.display_name} />
            </Field>
            <Button className="self-end" onClick={() => onRemove(index)} type="button" variant="outline">
              <Trash2Icon />
            </Button>
          </div>
        ))}
      </div>
      <Button onClick={onAdd} type="button" variant="outline">
        <PlusIcon data-icon="inline-start" />
        {t("nodes.addIP")}
      </Button>
    </FieldSet>
  );
}

export function NodeEnrollmentDeleteDialog({ onDeleted, onOpenChange, profile }: { onDeleted: () => Promise<void>; onOpenChange: (open: boolean) => void; profile: NodeEnrollmentProfile | null }) {
  const { locale, t } = useI18n();
  async function deleteProfile() {
    if (!profile) return;
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/node-enrollment-profiles/${profile.id}`);
      toast.success(t("nodes.enrollmentProfileDeleted"));
      await onDeleted();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }
  return (
    <Dialog onOpenChange={onOpenChange} open={Boolean(profile)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("nodes.deleteEnrollmentProfile")}</DialogTitle>
          <DialogDescription>{t("nodes.deleteEnrollmentProfileQuestion", { name: profile?.name ?? "" })}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} type="button" variant="outline">{t("common.cancel")}</Button>
          <Button onClick={() => void deleteProfile()} type="button" variant="destructive">{t("common.delete")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
