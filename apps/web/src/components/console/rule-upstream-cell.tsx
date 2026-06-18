"use client";

import { useI18n } from "@/components/console/i18n";
import type { Rule, Target, TargetGroup } from "@/components/console/types";

export function RuleUpstreamCell({
  rule,
  targetGroupOptionLabelsByID,
  targetGroupsByID,
  targetOptionLabelsByID,
  targetsByID,
}: {
  rule: Rule;
  targetGroupOptionLabelsByID: Map<string, string>;
  targetGroupsByID: Map<string, TargetGroup>;
  targetOptionLabelsByID: Map<string, string>;
  targetsByID: Map<string, Target>;
}) {
  const { t } = useI18n();
  if (rule.upstream.type === "TARGET") {
    const target = rule.upstream.target_id ? targetsByID.get(rule.upstream.target_id) : undefined;
    const fallbackLabel = rule.upstream.target_id ? targetOptionLabelsByID.get(rule.upstream.target_id) ?? rule.upstream.target_id : t("targets.unknownTarget");
    const fallbackAddress = addressFromOptionLabel(fallbackLabel);
    return (
      <div className="flex flex-col gap-1">
        <span>{target ? targetAddress(target) : fallbackAddress ?? fallbackLabel}</span>
        {target?.name ? <span className="text-xs text-muted-foreground">{target.name}</span> : null}
        {!target && fallbackAddress ? <span className="text-xs text-muted-foreground">{fallbackLabel}</span> : null}
      </div>
    );
  }

  const targetGroup = rule.upstream.target_group_id ? targetGroupsByID.get(rule.upstream.target_group_id) : undefined;
  const memberAddresses = safeTargetGroupMembers(targetGroup)
    .map((member) => targetsByID.get(member.target_id))
    .filter((target): target is Target => Boolean(target))
    .map(targetAddress);
  const fallbackLabel = rule.upstream.target_group_id ? targetGroupOptionLabelsByID.get(rule.upstream.target_group_id) ?? rule.upstream.target_group_id : t("targets.unknownTarget");

  return (
    <div className="flex flex-col gap-1">
      <span>{memberAddresses.length > 0 ? memberAddresses.join(", ") : fallbackLabel}</span>
      {targetGroup?.name ? <span className="text-xs text-muted-foreground">{targetGroup.name}</span> : null}
    </div>
  );
}

function safeTargetGroupMembers(group: TargetGroup | undefined) {
  return Array.isArray(group?.members) ? group.members : [];
}

function targetAddress(target: Target): string {
  return `${target.host}:${target.port}`;
}

function addressFromOptionLabel(label: string): string | null {
  return label.match(/\(([^()]+:\d+)\)/)?.[1] ?? null;
}
