"use client";

import { PencilIcon, PlusIcon } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { controlDelete, controlGet, controlPatch, controlPost, shortDate } from "@/components/console/control-api";
import { localizeControlError, localizeEnum, useI18n } from "@/components/console/i18n";
import { ResourceMultiSelect } from "@/components/console/resource-select";
import { EnumSelect, StatusBadge, TextAreaField, TextField } from "@/components/console/shared";
import type { NodeEnrollmentEvent, NodeEnrollmentProfile, NodeGroup } from "@/components/console/types";

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
  const groupOptions = groups.map((group) => ({ value: group.id, label: group.name }));
  const localizedProtocolOptions = protocolOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));
  const localizedDataplaneOptions = dataplaneModeOptions.map((option) => ({ value: option.value, label: t(option.labelKey) }));
  const ttlHours = profile?.expires_at ? String(Math.max(0, Math.ceil((Date.parse(profile.expires_at) - Date.now()) / 3600000))) : "720";

  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const listenIP = String(form.get("listen_ip") || "0.0.0.0");
    const listenIPLabel = String(form.get("listen_ip_label") || "default");
    const listenIPs = profile?.listen_ips?.length
      ? profile.listen_ips.map((item, index) => (index === 0 ? { ...item, listen_ip: listenIP, display_name: listenIPLabel } : item))
      : [{ listen_ip: listenIP, display_name: listenIPLabel }];
    const sendIP = String(form.get("send_ip") || "").trim();
    const sendIPLabel = String(form.get("send_ip_label") || sendIP).trim();
    const sendIPs = sendIP
      ? (profile?.send_ips?.length
        ? profile.send_ips.map((item, index) => (index === 0 ? { ...item, send_ip: sendIP, display_name: sendIPLabel } : item))
        : [{ send_ip: sendIP, display_name: sendIPLabel }])
      : [];
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
      listen_ips: listenIPs,
      send_ips: sendIPs,
      port_ranges: portRanges,
      max_rule_ports: Number(form.get("max_rule_ports") || profile?.max_rule_ports || 256),
      dns_publish_addresses: profile?.dns_publish_addresses ?? [],
      dataplane_mode: dataplaneMode,
      dataplane_conflict_policy: "FAIL_FAST",
      auto_update_enabled: form.get("auto_update_enabled") === "on",
      allowed_cidrs: String(form.get("allowed_cidrs") || "").split(/\s*,\s*|\n/).map((value) => value.trim()).filter(Boolean),
    };
    const submittedTTLHours = String(form.get("ttl_hours") || "");
    if (profile && submittedTTLHours === ttlHours) {
      payload.expires_at = profile.expires_at;
    } else {
      payload.ttl_hours = Number(submittedTTLHours || 720);
    }
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
        <TextField defaultValue={ttlHours} label={t("nodes.ttlHours")} name="ttl_hours" placeholder="720" type="number" />
        <TextField defaultValue={String(profile?.max_uses ?? 0)} label={t("nodes.maxUses")} name="max_uses" placeholder="0" required={false} type="number" />
      </div>
      <EnumSelect label={t("nodes.dataplaneMode")} onValueChange={setDataplaneMode} options={localizedDataplaneOptions} value={dataplaneMode} />
      <div className="grid gap-3 md:grid-cols-2">
        <TextField defaultValue={profile?.listen_ips?.[0]?.listen_ip ?? "0.0.0.0"} label={t("rules.listenIP")} name="listen_ip" placeholder="0.0.0.0" />
        <TextField defaultValue={profile?.listen_ips?.[0]?.display_name ?? "default"} label={t("nodes.listenIPLabel")} name="listen_ip_label" placeholder="default" />
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <TextField defaultValue={profile?.send_ips?.[0]?.send_ip ?? ""} label={t("nodes.sendIP")} name="send_ip" placeholder="198.51.100.10" required={false} />
        <TextField defaultValue={profile?.send_ips?.[0]?.display_name ?? ""} label={t("nodes.listenIPLabel")} name="send_ip_label" placeholder="primary" required={false} />
      </div>
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
