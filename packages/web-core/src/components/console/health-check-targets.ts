import type { HealthCheckTarget, HealthTargetScope, TargetGroup } from "./types";

export function formatHealthCheckTargets(targets: HealthCheckTarget[], targetGroups: TargetGroup[], targetScope?: HealthTargetScope): string {
  const concreteTargets = targets.filter((target) => target.target_id);
  if (concreteTargets.length > 0) {
    return concreteTargets.map((target) => `${target.target_name} (${target.target_host}:${target.target_port})`).join(", ");
  }
  const groupNames = new Map(targetGroups.map((group) => [group.id, group.name]));
  const seen = new Set<string>();
  const groups = targets
    .filter((target) => target.scope_type === "TARGET_GROUP" && target.target_group_id)
    .map((target) => target.target_group_id ?? "")
    .filter((targetGroupID) => {
      if (!targetGroupID || seen.has(targetGroupID)) {
        return false;
      }
      seen.add(targetGroupID);
      return true;
    })
    .map((targetGroupID) => groupNames.get(targetGroupID) ?? targetGroupID);
  if (targetScope?.type === "TARGET_GROUP" && targetScope.target_group_id && !seen.has(targetScope.target_group_id)) {
    groups.push(groupNames.get(targetScope.target_group_id) ?? targetScope.target_group_id);
  }
  return groups.join(", ");
}
