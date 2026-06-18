export function buildRuleUpstreamOptionPaths(): { targets: string; targetGroups: string } {
  return {
    targets: "/api/control/resource-options/targets",
    targetGroups: "/api/control/resource-options/target-groups",
  };
}
