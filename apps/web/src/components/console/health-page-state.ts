import { hasAnyPermission, hasPermission } from "@/components/console/permissions";
import type { ControlSession } from "@/components/console/types";

type ResourceState = {
  loading: boolean;
  error: string;
};

export function canReadHealthChecks(session: ControlSession | null): boolean {
  return hasPermission(session, "health_checks.read");
}

export function canUseHealthCheckEditor(session: ControlSession | null): boolean {
  return hasPermission(session, "health_checks.manage")
    && hasAnyPermission(session, ["targets.read", "targets.manage"])
    && hasPermission(session, "monitors.read");
}

export function healthPageResourceState(input: {
  checks: ResourceState;
  targets: ResourceState;
  targetGroups: ResourceState;
  monitors: ResourceState;
  monitorGroups: ResourceState;
  includeEditorDependencies: boolean;
}): ResourceState {
  return {
    loading: input.checks.loading
      || (input.includeEditorDependencies && (input.targets.loading || input.targetGroups.loading || input.monitors.loading || input.monitorGroups.loading)),
    error: input.checks.error
      || (input.includeEditorDependencies ? input.targets.error || input.targetGroups.error || input.monitors.error || input.monitorGroups.error : ""),
  };
}
