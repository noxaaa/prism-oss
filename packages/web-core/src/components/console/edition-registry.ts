import {
  ActivityIcon,
  GlobeIcon,
  HeartPulseIcon,
  LayoutDashboardIcon,
  NetworkIcon,
  RadarIcon,
  RouteIcon,
  ServerIcon,
  SettingsIcon,
  TargetIcon,
} from "lucide-react";
import type { ComponentType, SVGProps } from "react";
import type { MessageKey } from "@noxaaa/prism-oss-web-core/console/i18n";

export enum Capability {
  CoreForwarding = "core_forwarding",
  Targets = "targets",
  Rules = "rules",
  Nodes = "nodes",
  Monitors = "monitors",
  HealthChecks = "health_checks",
  DNS = "dns",
  BasicMetrics = "basic_metrics",
  SingleUserAuth = "single_user_auth",
}

export type Workspace = "admin" | "user";
export type ConsoleEdition = "oss" | "full";

export type ConsoleNavItem = {
  href: string;
  icon: ComponentType<SVGProps<SVGSVGElement>>;
  key: string;
  labelKey: MessageKey;
  permissions: string[];
  requiredPermissions?: string[];
};

export type ConsoleRegistry = {
  adminWorkspacePermissions: string[];
  capabilities: string[];
  itemsByWorkspace: Record<Workspace, ConsoleNavItem[]>;
};

export type ConsoleExtension = {
  adminWorkspacePermissions?: string[];
  capabilities?: string[];
  itemsByWorkspace?: Partial<Record<Workspace, ConsoleNavItem[]>>;
};

function uniqueValues(values: string[]): string[] {
  return [...new Set(values)];
}

export function createConsoleRegistry(base: ConsoleRegistry = ossConsoleRegistry, extensions: ConsoleExtension[] = []): ConsoleRegistry {
  return mergeConsoleExtensions(base, ...extensions);
}

export function mergeConsoleExtensions(base: ConsoleRegistry, ...extensions: ConsoleExtension[]): ConsoleRegistry {
  const next: ConsoleRegistry = {
    adminWorkspacePermissions: [...base.adminWorkspacePermissions],
    capabilities: [...base.capabilities],
    itemsByWorkspace: {
      admin: [...base.itemsByWorkspace.admin],
      user: [...base.itemsByWorkspace.user],
    },
  };

  for (const extension of extensions) {
    next.adminWorkspacePermissions.push(...(extension.adminWorkspacePermissions ?? []));
    next.capabilities.push(...(extension.capabilities ?? []));
    for (const workspace of ["admin", "user"] as const) {
      next.itemsByWorkspace[workspace].push(...(extension.itemsByWorkspace?.[workspace] ?? []));
    }
  }

  return {
    adminWorkspacePermissions: uniqueValues(next.adminWorkspacePermissions),
    capabilities: uniqueValues(next.capabilities),
    itemsByWorkspace: next.itemsByWorkspace,
  };
}

export const overviewItem: ConsoleNavItem = {
  href: "/console/admin/overview",
  icon: LayoutDashboardIcon,
  key: "overview",
  labelKey: "nav.overview",
  permissions: ["nodes.read", "nodes.manage", "targets.read", "targets.manage", "rules.read_all", "rules.manage_all", "rules.manage_own"],
};

export const coreAdminItems: ConsoleNavItem[] = [
  overviewItem,
  { href: "/console/admin/nodes", icon: ServerIcon, key: "nodes", labelKey: "nav.nodes", permissions: ["nodes.read", "nodes.manage"] },
  { href: "/console/admin/monitors", icon: RadarIcon, key: "monitors", labelKey: "nav.monitors", permissions: ["monitors.read"] },
  { href: "/console/admin/health", icon: HeartPulseIcon, key: "health", labelKey: "nav.health", permissions: ["health_checks.read"] },
  { href: "/console/admin/dns", icon: GlobeIcon, key: "dns", labelKey: "nav.dns", permissions: ["dns.read", "dns.manage"] },
  { href: "/console/admin/targets", icon: TargetIcon, key: "targets", labelKey: "nav.targets", permissions: ["targets.read", "targets.manage"] },
  { href: "/console/admin/rules", icon: RouteIcon, key: "rules", labelKey: "nav.rules", permissions: ["rules.read_all", "rules.manage_all", "rules.manage_own"] },
  { href: "/console/admin/settings", icon: SettingsIcon, key: "settings", labelKey: "nav.settings", permissions: ["organization.read", "organization.update"] },
];

export const coreUserItems: ConsoleNavItem[] = [
  { href: "/console/user/rules", icon: RouteIcon, key: "rules", labelKey: "nav.myRules", permissions: ["rules.read_own", "rules.manage_own"] },
  { href: "/console/user/usage", icon: ActivityIcon, key: "usage", labelKey: "nav.usage", permissions: ["rules.read_own", "rules.manage_own"], requiredPermissions: ["traffic.read_own"] },
  { href: "/console/user/nodes", icon: NetworkIcon, key: "nodes", labelKey: "nav.availableNodes", permissions: ["nodes.read"] },
  { href: "/console/user/targets", icon: TargetIcon, key: "targets", labelKey: "nav.targets", permissions: ["targets.read", "targets.manage"] },
];

export const ossConsoleRegistry: ConsoleRegistry = {
  adminWorkspacePermissions: [
    "audit_logs.read",
    "nodes.read",
    "nodes.manage",
    "monitors.read",
    "monitors.manage",
    "health_checks.read",
    "health_checks.manage",
    "dns.read",
    "dns.manage",
    "organization.update",
    "quotas.manage",
    "rules.read_all",
    "rules.manage_all",
    "targets.read",
    "targets.manage",
    "traffic.read_all",
  ],
  capabilities: [
    Capability.CoreForwarding,
    Capability.Targets,
    Capability.Rules,
    Capability.Nodes,
    Capability.Monitors,
    Capability.HealthChecks,
    Capability.DNS,
    Capability.BasicMetrics,
    Capability.SingleUserAuth,
  ],
  itemsByWorkspace: {
    admin: coreAdminItems,
    user: coreUserItems,
  },
};

export function consoleEditionFromEnv(value = process.env.NEXT_PUBLIC_PRISM_EDITION): ConsoleEdition {
  if (value === undefined || value === "") {
    return "oss";
  }
  if (value === "oss" || value === "full") {
    return value;
  }
  throw new Error(`Unsupported NEXT_PUBLIC_PRISM_EDITION: ${value}`);
}

consoleEditionFromEnv();
export const defaultConsoleRegistry = ossConsoleRegistry;
