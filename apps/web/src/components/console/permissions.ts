import type { ControlSession } from "@/components/console/types";
import { defaultConsoleRegistry } from "@/components/console/edition-registry";

const adminPermissions = new Set(defaultConsoleRegistry.adminWorkspacePermissions);

export function hasPermission(session: ControlSession | null, permission: string): boolean {
  return Boolean(session?.permissions?.includes(permission));
}

export function hasAnyPermission(session: ControlSession | null, permissions: string[]): boolean {
  return permissions.some((permission) => hasPermission(session, permission));
}

export function canUseAdminWorkspace(session: ControlSession | null): boolean {
  return Boolean(session?.permissions?.some((permission) => adminPermissions.has(permission)));
}

export function roleSummary(session: ControlSession | null): string {
  const roles = session?.roles ?? [];
  if (roles.length === 0) {
    return "";
  }
  return roles.map((role) => role.name || role.key).join(", ");
}
