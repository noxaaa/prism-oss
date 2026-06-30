"use client";

import { EditIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { useEffect, useMemo, useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@noxaaa/prism-oss-web-core/ui/button";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@noxaaa/prism-oss-web-core/ui/field";
import { Input } from "@noxaaa/prism-oss-web-core/ui/input";
import { Switch } from "@noxaaa/prism-oss-web-core/ui/switch";
import { controlPatch, controlPost } from "@noxaaa/prism-oss-web-core/console/control-api";
import { localizeControlError, localizeEnum, useI18n } from "@noxaaa/prism-oss-web-core/console/i18n";
import { ResourceSelect } from "@noxaaa/prism-oss-web-core/console/resource-select";
import { ControlledTextField, EnumSelect, FieldRequirementBadge, TextField, ensureFirstValue, useControlList } from "@noxaaa/prism-oss-web-core/console/shared";
import { cn } from "@noxaaa/prism-oss-web-core/lib/utils";
import type { ResourceOption, Rule, RulePortSegment } from "@noxaaa/prism-oss-web-core/console/types";

const ruleProtocolOptions = [{ value: "TCP", label: "TCP" }, { value: "UDP", label: "UDP" }, { value: "TCP_UDP", label: "TCP + UDP" }];
const forwardingTypeOptions = [{ value: "DIRECT", label: "Direct forwarding" }];
const ruleFailurePolicyOptions = [{ value: "KEEP_ENABLED", labelKey: "rules.failurePolicyKeepEnabled" }, { value: "DISABLE_WHEN_ALL_NODES_FAILED", labelKey: "rules.failurePolicyDisableAllFailed" }];
const ruleDataplanePreferenceOptions = [{ value: "AUTO", labelKey: "rules.dataplaneAuto" }, { value: "NATIVE", labelKey: "rules.dataplaneNative" }, { value: "HAPROXY", labelKey: "rules.dataplaneHAProxy" }, { value: "NFTABLES", labelKey: "rules.dataplaneNFTables" }];
const defaultSendIPOptionValue = "__system_default_send_ip__";
const proxyProtocolOptions = [
  { value: "NONE", label: "None" },
  { value: "V1", label: "Proxy Protocol v1" },
  { value: "V2", label: "Proxy Protocol v2" },
];

export function RuleMutationForm({ onSaved, rule, submitLabel }: { onSaved: () => Promise<void>; rule?: Rule; submitLabel: string }) {
  const { locale, t } = useI18n();
  const [protocol, setProtocol] = useState(rule?.protocol ?? "TCP");
  const [portSegments, setPortSegments] = useState<RulePortSegment[]>(normalizedInitialPortSegments(rule));
  const [nodeGroupID, setNodeGroupID] = useState(rule?.node_group_id ?? "");
  const [listenIP, setListenIP] = useState(rule?.listen_ip ?? "");
  const [sendIP, setSendIP] = useState(rule?.send_ip ?? "");
  const [upstreamType, setUpstreamType] = useState(rule?.upstream.type ?? "TARGET_GROUP");
  const [targetID, setTargetID] = useState(rule?.upstream.target_id ?? "");
  const [targetGroupID, setTargetGroupID] = useState(rule?.upstream.target_group_id ?? "");
  const [matchType, setMatchType] = useState(rule?.match.type ?? "ANY_INBOUND");
  const [proxyIn, setProxyIn] = useState(rule?.proxy_protocol.in ?? "NONE");
  const [proxyOut, setProxyOut] = useState(rule?.proxy_protocol.out ?? "NONE");
  const [failurePolicy, setFailurePolicy] = useState(rule?.failure_policy ?? "KEEP_ENABLED");
  const [dataplanePreference, setDataplanePreference] = useState(rule?.dataplane_preference ?? "AUTO");
  const [ruleEnabled, setRuleEnabled] = useState(rule?.enabled ?? false);
  const nodeGroups = useControlList<ResourceOption>("/api/control/resource-options/node-groups?access=USE");
  const targets = useControlList<ResourceOption>("/api/control/resource-options/targets");
  const targetGroups = useControlList<ResourceOption>("/api/control/resource-options/target-groups");
  const listenIPOptionsURL = useMemo(() => buildListenIPOptionsURL(nodeGroupID, protocol, normalizePortSegments(portSegments)), [nodeGroupID, portSegments, protocol]);
  const listenIPs = useControlList<ResourceOption>(
    listenIPOptionsURL,
  );
  const sendIPs = useControlList<ResourceOption>(
    nodeGroupID ? `/api/control/resource-options/node-group-send-ips?node_group_id=${encodeURIComponent(nodeGroupID)}` : "",
  );
  const sendIPOptions = useMemo(() => [{ value: defaultSendIPOptionValue, label: t("rules.defaultSendIP") }, ...sendIPs.data], [sendIPs.data, t]);
  const sendIPSelectValue = sendIP === "" ? defaultSendIPOptionValue : sendIP;
  const localizedProtocolOptions = ruleProtocolOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));
  const localizedForwardingTypeOptions = forwardingTypeOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));
  const localizedProxyProtocolOptions = proxyProtocolOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));
  const localizedFailurePolicyOptions = ruleFailurePolicyOptions.map((option) => ({ value: option.value, label: t(option.labelKey) }));
  const localizedDataplanePreferenceOptions = ruleDataplanePreferenceOptions.map((option) => ({ value: option.value, label: t(option.labelKey) }));
  const localizedMatchOptions = protocol !== "TCP"
    ? [{ value: "ANY_INBOUND", label: localizeEnum("ANY_INBOUND", locale) }]
    : [{ value: "ANY_INBOUND", label: localizeEnum("ANY_INBOUND", locale) }, { value: "TLS_SNI", label: localizeEnum("TLS_SNI", locale) }];
  const localizedUpstreamOptions = [{ value: "TARGET_GROUP", label: localizeEnum("TARGET_GROUP", locale) }, { value: "TARGET", label: localizeEnum("TARGET", locale) }];

  useEffect(() => {
    if (rule) {
      return;
    }
    ensureFirstValue(nodeGroups.data, nodeGroupID, setNodeGroupID);
    ensureFirstValue(targets.data, targetID, setTargetID);
    ensureFirstValue(targetGroups.data, targetGroupID, setTargetGroupID);
    ensureFirstValue(listenIPs.data, listenIP, setListenIP);
  }, [listenIPs.data, listenIP, nodeGroupID, nodeGroups.data, rule, targetGroupID, targetGroups.data, targetID, targets.data]);

  useEffect(() => {
    if (sendIPs.loading) {
      return;
    }
    if (rule && sendIP === (rule.send_ip ?? "")) {
      return;
    }
    if (!sendIPOptions.some((option) => option.value === (sendIP === "" ? defaultSendIPOptionValue : sendIP))) {
      setSendIP("");
    }
  }, [rule, sendIP, sendIPOptions, sendIPs.loading]);

  useEffect(() => {
    if (protocol !== "TCP" && matchType !== "ANY_INBOUND") {
      setMatchType("ANY_INBOUND");
    }
  }, [matchType, protocol]);

  useEffect(() => {
    if (protocol === "UDP") {
      setProxyIn("NONE");
      setProxyOut("NONE");
    }
  }, [protocol]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const normalizedSegments = normalizePortSegments(portSegments);
    const body = {
      name: form.get("name"),
      tags: String(form.get("tags") ?? "").split(",").map((tag) => tag.trim()).filter(Boolean),
      node_group_id: nodeGroupID,
      listen_ip: listenIP,
      failure_policy: failurePolicy,
      dataplane_preference: dataplanePreference,
      forwarding_type: "DIRECT",
      protocol,
      port: legacyPortForSegments(normalizedSegments),
      port_segments: normalizedSegments,
      send_ip: sendIP === defaultSendIPOptionValue ? "" : sendIP,
      match: {
        type: matchType,
        sni_hostname: matchType === "TLS_SNI" ? form.get("sni_hostname") : "",
      },
      proxy_protocol: { in: proxyIn, out: proxyOut },
      upstream: upstreamType === "TARGET" ? { type: "TARGET", target_id: targetID } : { type: "TARGET_GROUP", target_group_id: targetGroupID },
      enabled: ruleEnabled,
    };
    try {
      if (rule) {
        await controlPatch<Rule>(`/api/control/rules/${rule.id}`, body);
        toast.success(t("rules.updated"));
      } else {
        await controlPost<Rule>("/api/control/rules", body);
        formElement.reset();
        setPortSegments([{ start_port: 443, end_port: 443 }]);
        setRuleEnabled(false);
        toast.success(t("rules.created"));
      }
      await onSaved();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  const canSubmit = nodeGroupID && listenIP && portSegments.length > 0 && (upstreamType === "TARGET" ? targetID : targetGroupID);

  return (
    <form className="flex flex-col gap-5" onSubmit={submit}>
      <FieldGroup>
        <TextField defaultValue={rule?.name} label={t("field.name")} name="name" placeholder="customer-a-https" />
        <TextField defaultValue={safeArray(rule?.tags).join(",")} label={t("field.tags")} name="tags" placeholder="customer-a,https" required={false} />
        <ResourceSelect label={t("rules.nodeGroup")} onValueChange={(value) => { setNodeGroupID(value); setListenIP(""); setSendIP(""); }} options={nodeGroups.data} value={nodeGroupID} />
        <EnumSelect label={t("rules.forwardingType")} onValueChange={() => undefined} options={localizedForwardingTypeOptions} value="DIRECT" />
        <EnumSelect label={t("rules.protocol")} onValueChange={(value) => { setProtocol(value); setListenIP(""); }} options={localizedProtocolOptions} value={protocol} />
        <PortSegmentsEditor onChange={(segments) => { setPortSegments(segments); setListenIP(""); }} segments={portSegments} />
        <ResourceSelect label={t("rules.listenIP")} onValueChange={setListenIP} options={listenIPs.data} value={listenIP} />
        <ResourceSelect label={t("rules.sendIP")} onValueChange={(value) => setSendIP(value === defaultSendIPOptionValue ? "" : value)} options={sendIPOptions} value={sendIPSelectValue} />
        <EnumSelect label={t("rules.failurePolicy")} onValueChange={setFailurePolicy} options={localizedFailurePolicyOptions} value={failurePolicy} />
        <FieldDescription>{t("rules.failurePolicyDescription")}</FieldDescription>
        <EnumSelect label={t("rules.dataplanePreference")} onValueChange={setDataplanePreference} options={localizedDataplanePreferenceOptions} value={dataplanePreference} />
        <FieldDescription>{t("rules.dataplanePreferenceDescription")}</FieldDescription>
        <div className={cn("grid gap-3", matchType === "TLS_SNI" ? "md:grid-cols-2" : "")}>
          <EnumSelect label={t("rules.matchType")} onValueChange={setMatchType} options={localizedMatchOptions} value={matchType} />
          {matchType === "TLS_SNI" ? <TextField defaultValue={rule?.match.sni_hostname ?? ""} label={t("rules.sniHostname")} name="sni_hostname" placeholder="app.customer.example" /> : null}
        </div>
        {protocol !== "UDP" ? (
          <div className="grid gap-3 md:grid-cols-2">
            <EnumSelect label={t("rules.proxyProtocolIn")} onValueChange={setProxyIn} options={localizedProxyProtocolOptions} value={proxyIn} />
            <EnumSelect label={t("rules.proxyProtocolOut")} onValueChange={setProxyOut} options={localizedProxyProtocolOptions} value={proxyOut} />
          </div>
        ) : null}
        <EnumSelect label={t("rules.upstreamType")} onValueChange={setUpstreamType} options={localizedUpstreamOptions} value={upstreamType} />
        {upstreamType === "TARGET" ? (
          <ResourceSelect label={t("rules.target")} onValueChange={setTargetID} options={targets.data} value={targetID} />
        ) : (
          <ResourceSelect label={t("rules.targetGroup")} onValueChange={setTargetGroupID} options={targetGroups.data} value={targetGroupID} />
        )}
        <Field orientation="horizontal">
          <Switch checked={ruleEnabled} id="rule_enabled" onCheckedChange={setRuleEnabled} />
          <FieldLabel htmlFor="rule_enabled">{t("common.enabled")}</FieldLabel>
        </Field>
      </FieldGroup>
      <Button disabled={!canSubmit} type="submit">
        {rule ? <EditIcon data-icon="inline-start" /> : <PlusIcon data-icon="inline-start" />}
        {submitLabel}
      </Button>
    </form>
  );
}

export function PortSegmentsEditor({ onChange, segments }: { onChange: (segments: RulePortSegment[]) => void; segments: RulePortSegment[] }) {
  const { t } = useI18n();
  const normalized = normalizePortSegments(segments);
  function update(index: number, field: keyof RulePortSegment, value: string) {
    onChange(segments.map((segment, itemIndex) => (itemIndex === index ? { ...segment, [field]: Number(value) || 0 } : segment)));
  }
  return (
    <Field>
      <FieldLabel>{t("rules.portSegments")}<FieldRequirementBadge required /></FieldLabel>
      <div className="flex flex-col gap-3">
        {segments.map((segment, index) => (
          <div className="grid gap-3 md:grid-cols-[1fr_1fr_auto]" key={index}>
            <ControlledTextField label={t("nodes.startPort")} onValueChange={(value) => update(index, "start_port", value)} placeholder="443" type="number" value={String(segment.start_port || "")} />
            <ControlledTextField label={t("nodes.endPort")} onValueChange={(value) => update(index, "end_port", value)} placeholder="443" type="number" value={String(segment.end_port || "")} />
            <Button className="self-end" disabled={segments.length === 1} onClick={() => onChange(segments.filter((_, itemIndex) => itemIndex !== index))} type="button" variant="outline">
              <Trash2Icon />
            </Button>
          </div>
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Button onClick={() => onChange([...segments, { start_port: 443, end_port: 443 }])} type="button" variant="outline">
          <PlusIcon data-icon="inline-start" />
          {t("rules.addPortSegment")}
        </Button>
        <FieldDescription>{formatPortSegments(normalized)} · {t("rules.expandedPortCount", { count: expandedPortCount(normalized) })}</FieldDescription>
      </div>
    </Field>
  );
}

export function formatPortSegments(segments: RulePortSegment[] | undefined): string {
  const normalized = normalizePortSegments(segments ?? []);
  if (normalized.length === 0) {
    return "";
  }
  return normalized.map((segment) => (segment.start_port === segment.end_port ? String(segment.start_port) : `${segment.start_port}-${segment.end_port}`)).join(", ");
}

function normalizedInitialPortSegments(rule?: Rule): RulePortSegment[] {
  if (rule?.port_segments?.length) {
    return normalizePortSegments(rule.port_segments);
  }
  return [{ start_port: rule?.port ?? 443, end_port: rule?.port ?? 443 }];
}

function normalizePortSegments(segments: RulePortSegment[]): RulePortSegment[] {
  const cleaned = segments
    .map((segment) => ({
      start_port: Math.max(0, Math.min(65535, Number(segment.start_port) || 0)),
      end_port: Math.max(0, Math.min(65535, Number(segment.end_port) || 0)),
    }))
    .filter((segment) => segment.start_port > 0 && segment.end_port > 0)
    .map((segment) => segment.start_port <= segment.end_port ? segment : { start_port: segment.end_port, end_port: segment.start_port })
    .sort((left, right) => left.start_port - right.start_port || left.end_port - right.end_port);
  const result: RulePortSegment[] = [];
  for (const segment of cleaned) {
    const previous = result[result.length - 1];
    if (previous && segment.start_port <= previous.end_port + 1) {
      previous.end_port = Math.max(previous.end_port, segment.end_port);
    } else {
      result.push({ ...segment });
    }
  }
  return result;
}

function expandedPortCount(segments: RulePortSegment[]): number {
  return segments.reduce((total, segment) => total + Math.max(0, segment.end_port - segment.start_port + 1), 0);
}

function buildListenIPOptionsURL(nodeGroupID: string, protocol: string, segments: RulePortSegment[]): string {
  if (!nodeGroupID) {
    return "";
  }
  const params = new URLSearchParams({ node_group_id: nodeGroupID });
  if (protocol && segments.length > 0) {
    params.set("protocol", protocol);
    params.set("port_segments", formatPortSegments(segments));
  }
  return `/api/control/resource-options/node-group-listen-ips?${params.toString()}`;
}

function legacyPortForSegments(segments: RulePortSegment[]): number {
  if (segments.length === 1 && segments[0].start_port === segments[0].end_port) {
    return segments[0].start_port;
  }
  return 0;
}

function safeArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}
