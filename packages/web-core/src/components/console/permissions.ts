import type { ControlSession } from "@noxaaa/prism-oss-web-core/console/types";
import { defaultConsoleRegistry, type ConsoleRegistry } from "@noxaaa/prism-oss-web-core/console/edition-registry";

export function hasPermission(session: ControlSession | null, permission: string): boolean {
  return Boolean(session?.permissions?.includes(permission));
}

export function hasAnyPermission(session: ControlSession | null, permissions: string[]): boolean {
  return permissions.some((permission) => hasPermission(session, permission));
}

export function canUseAdminWorkspace(session: ControlSession | null, registry: Pick<ConsoleRegistry, "adminWorkspacePermissions"> = defaultConsoleRegistry): boolean {
  const adminPermissions = new Set(registry.adminWorkspacePermissions);
  return Boolean(session?.permissions?.some((permission) => adminPermissions.has(permission)));
}

export function roleSummary(session: ControlSession | null): string {
  const roles = session?.roles ?? [];
  if (roles.length === 0) {
    return "";
  }
  return roles.map((role) => role.name || role.key).join(", ");
}
