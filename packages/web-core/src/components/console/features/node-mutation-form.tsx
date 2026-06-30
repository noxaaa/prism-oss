"use client";

import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@noxaaa/prism-oss-web-core/ui/button";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldSet, FieldLegend } from "@noxaaa/prism-oss-web-core/ui/field";
import { Input } from "@noxaaa/prism-oss-web-core/ui/input";
import { controlPatch, controlPost } from "@noxaaa/prism-oss-web-core/console/control-api";
import { localizeControlError, localizeEnum, useI18n } from "@noxaaa/prism-oss-web-core/console/i18n";
import { ResourceMultiSelect } from "@noxaaa/prism-oss-web-core/console/resource-select";
import { EnumSelect, FieldRequirementBadge, TextAreaField, TextField } from "@noxaaa/prism-oss-web-core/console/shared";
import type { NodeGroup, NodeResource, NodeSendIP } from "@noxaaa/prism-oss-web-core/console/types";

const protocolOptions = [
  { value: "TCP", label: "TCP" },
  { value: "UDP", label: "UDP" },
  { value: "TCP_UDP", label: "TCP + UDP" },
];

const dataplaneModeOptions = [
  { value: "AUTO", labelKey: "nodes.dataplaneAuto" },
  { value: "NATIVE", labelKey: "nodes.dataplaneNative" },
  { value: "HAPROXY", labelKey: "nodes.dataplaneHAProxy" },
  { value: "NFTABLES", labelKey: "nodes.dataplaneNFTables" },
];

type EditableIP = {
  address: string;
  display_name: string;
};

export function NodeMutationForm({ groups, node, onSaved }: { groups: NodeGroup[]; node?: NodeResource; onSaved: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const initialProtocol = initialPortProtocol(node);
  const initialPortRange = node?.port_ranges[0];
  const initialStartPort = initialPortRange?.start_port ? String(initialPortRange.start_port) : "";
  const initialEndPort = initialPortRange?.end_port ? String(initialPortRange.end_port) : "";
  const [nodeGroupIDs, setNodeGroupIDs] = useState<string[]>(node?.group_ids ?? []);
  const [protocol, setProtocol] = useState(initialProtocol);
  const [dataplaneMode, setDataplaneMode] = useState(node?.dataplane_mode ?? "AUTO");
  const [listenIPs, setListenIPs] = useState<EditableIP[]>(
    node?.listen_ips?.length ? node.listen_ips.map((item) => ({ address: item.listen_ip, display_name: item.display_name })) : [{ address: "0.0.0.0", display_name: "default" }],
  );
  const [sendIPs, setSendIPs] = useState<EditableIP[]>(toEditableSendIPs(node?.send_ips));
  const groupOptions = groups.map((group) => ({ value: group.id, label: group.name }));
  const localizedProtocolOptions = protocolOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));
  const localizedDataplaneOptions = dataplaneModeOptions.map((option) => ({ value: option.value, label: t(option.labelKey) }));

  function updateIP(kind: "listen" | "send", index: number, field: keyof EditableIP, value: string) {
    const setItems = kind === "listen" ? setListenIPs : setSendIPs;
    setItems((current) => current.map((item, itemIndex) => (itemIndex === index ? { ...item, [field]: value } : item)));
  }

  function addIP(kind: "listen" | "send") {
    const setItems = kind === "listen" ? setListenIPs : setSendIPs;
    setItems((current) => [...current, { address: "", display_name: "" }]);
  }

  function removeIP(kind: "listen" | "send", index: number) {
    const setItems = kind === "listen" ? setListenIPs : setSendIPs;
    setItems((current) => current.filter((_, itemIndex) => itemIndex !== index));
  }

  async function saveNode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const normalizedListenIPs = listenIPs
      .map((item) => ({ listen_ip: item.address.trim(), display_name: item.display_name.trim() }))
      .filter((item) => item.listen_ip !== "");
    if (normalizedListenIPs.length === 0) {
      normalizedListenIPs.push({ listen_ip: "0.0.0.0", display_name: "default" });
    }
    const normalizedSendIPs = sendIPs
      .map((item) => ({ send_ip: item.address.trim(), display_name: item.display_name.trim() }))
      .filter((item) => item.send_ip !== "");
    const startPortValue = String(form.get("start_port") ?? "");
    const endPortValue = String(form.get("end_port") ?? "");
    const portControlsChanged = protocol !== initialProtocol || startPortValue !== initialStartPort || endPortValue !== initialEndPort;
    const portRanges = node && node.port_ranges.length > 0 && !portControlsChanged
      ? node.port_ranges.map((range) => ({ protocol: range.protocol, start_port: range.start_port, end_port: range.end_port }))
      : nodePortRangesForSelection(protocol, Number(startPortValue || 10000), Number(endPortValue || 20000));
    try {
      const publishAddress = String(form.get("dns_publish_address") ?? "").trim();
      const initialManualPublishAddress = node?.dns_publish_addresses?.find((address) => address.source === "MANUAL")?.address ?? "";
      const publishAddressChanged = !node || publishAddress !== initialManualPublishAddress;
      const payload: Record<string, unknown> = {
        name: form.get("name"),
        public_description: form.get("public_description"),
        group_ids: nodeGroupIDs,
        listen_ips: normalizedListenIPs,
        send_ips: normalizedSendIPs,
        port_ranges: portRanges,
        max_rule_ports: Number(form.get("max_rule_ports") || node?.max_rule_ports || 256),
        dataplane_mode: dataplaneMode,
        dataplane_conflict_policy: "FAIL_FAST",
      };
      if (publishAddressChanged) {
        payload.dns_publish_addresses = nodeDNSPublishAddressPayload(node, publishAddress);
      }
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
        <EnumSelect label={t("nodes.dataplaneMode")} onValueChange={setDataplaneMode} options={localizedDataplaneOptions} value={dataplaneMode} />
        <FieldDescription>{t("nodes.dataplaneModeDescription")}</FieldDescription>
        <TextField defaultValue={node?.dns_publish_addresses?.find((address) => address.source === "MANUAL")?.address ?? ""} label={t("nodes.dnsPublishAddress")} name="dns_publish_address" placeholder="203.0.113.10" required={false} />
        <IPListEditor items={listenIPs} kind="listen" legend={t("nodes.listenIPs")} description={t("nodes.listenIPsDescription")} label={t("rules.listenIP")} labelPlaceholder="default" onAdd={() => addIP("listen")} onRemove={(index) => removeIP("listen", index)} onUpdate={(index, field, value) => updateIP("listen", index, field, value)} />
        <IPListEditor items={sendIPs} kind="send" legend={t("nodes.sendIPs")} description={t("nodes.sendIPsDescription")} label={t("nodes.sendIP")} labelPlaceholder="primary" onAdd={() => addIP("send")} onRemove={(index) => removeIP("send", index)} onUpdate={(index, field, value) => updateIP("send", index, field, value)} />
        <EnumSelect label={t("rules.protocol")} onValueChange={setProtocol} options={localizedProtocolOptions} value={protocol} />
        <div className="grid gap-3 md:grid-cols-3">
          <TextField defaultValue={initialStartPort} label={t("nodes.startPort")} name="start_port" placeholder="10000" required={false} type="number" />
          <TextField defaultValue={initialEndPort} label={t("nodes.endPort")} name="end_port" placeholder="20000" required={false} type="number" />
          <TextField defaultValue={String(node?.max_rule_ports ?? 256)} label={t("nodes.maxRulePorts")} name="max_rule_ports" placeholder="256" required={false} type="number" />
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

function IPListEditor({
  description,
  items,
  kind,
  label,
  labelPlaceholder,
  onAdd,
  onRemove,
  onUpdate,
  legend,
}: {
  description: string;
  items: EditableIP[];
  kind: "listen" | "send";
  label: string;
  labelPlaceholder: string;
  onAdd: () => void;
  onRemove: (index: number) => void;
  onUpdate: (index: number, field: keyof EditableIP, value: string) => void;
  legend: string;
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
              <FieldLabel>{label}<FieldRequirementBadge required={kind === "listen"} /></FieldLabel>
              <Input onChange={(event) => onUpdate(index, "address", event.target.value)} placeholder="0.0.0.0" value={item.address} />
            </Field>
            <Field>
              <FieldLabel>{kind === "listen" ? t("nodes.listenIPLabel") : t("nodes.sendIPLabel")}<FieldRequirementBadge required={false} /></FieldLabel>
              <Input onChange={(event) => onUpdate(index, "display_name", event.target.value)} placeholder={labelPlaceholder} value={item.display_name} />
            </Field>
            <Button className="self-end" disabled={items.length === 0} onClick={() => onRemove(index)} type="button" variant="outline">
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

function nodeDNSPublishAddressPayload(node: NodeResource | undefined, primaryAddress: string) {
  if (!primaryAddress) {
    return [];
  }
  const existing = (node?.dns_publish_addresses ?? []).find((address) => address.source === "MANUAL" && address.address === primaryAddress);
  return [{
    address_type: existing?.address_type ?? "",
    address: primaryAddress,
    enabled: existing?.enabled ?? true,
  }];
}

function initialPortProtocol(node?: NodeResource) {
  const protocols = new Set((node?.port_ranges ?? []).map((range) => range.protocol));
  if (protocols.has("TCP") && protocols.has("UDP")) {
    return "TCP_UDP";
  }
  return node?.port_ranges[0]?.protocol ?? "TCP";
}

function toEditableSendIPs(sendIPs: NodeSendIP[] | undefined): EditableIP[] {
  return sendIPs?.length ? sendIPs.map((item) => ({ address: item.send_ip, display_name: item.display_name })) : [];
}

function nodePortRangesForSelection(protocol: string, startPort: number, endPort: number) {
  if (protocol === "TCP_UDP") {
    return [
      { protocol: "TCP", start_port: startPort, end_port: endPort },
      { protocol: "UDP", start_port: startPort, end_port: endPort },
    ];
  }
  return [{ protocol, start_port: startPort, end_port: endPort }];
}
