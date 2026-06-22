import { hasAnyPermission } from "@/components/console/permissions";
import type { ControlSession } from "@/components/console/types";

type ResourceState = {
  loading: boolean;
  error: string;
};

export function canUseDNSHealthSelector(session: ControlSession | null): boolean {
  return hasAnyPermission(session, ["health_checks.read", "health_checks.manage"]);
}

export function dnsPageResourceState(input: {
  credentials: ResourceState;
  records: ResourceState;
  healthChecks: ResourceState;
  includeHealthChecks: boolean;
}): ResourceState {
  return {
    loading: input.records.loading || input.credentials.loading || (input.includeHealthChecks && input.healthChecks.loading),
    error: input.records.error || input.credentials.error || (input.includeHealthChecks ? input.healthChecks.error : ""),
  };
}
