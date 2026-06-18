import { describe, expect, it } from "vitest";
import { canUseAdminWorkspace, hasPermission, roleSummary } from "./permissions";
import type { ControlSession } from "./types";

describe("console permissions", () => {
  const baseSession: ControlSession = {
    user: { id: "user_01", email: "user@example.com", name: "User" },
    permissions: ["organization.read", "rules.manage_own"],
    roles: [{ id: "role_01", key: "user", name: "User", description: "", is_system: true, permissions: [], resource_scopes: [] }],
    resource_scopes: [],
  };

  it("keeps normal users in the user workspace by default", () => {
    expect(canUseAdminWorkspace(baseSession)).toBe(false);
    expect(hasPermission(baseSession, "rules.manage_own")).toBe(true);
  });

  it("treats admin capability as permission-driven, not role-name-driven", () => {
    expect(canUseAdminWorkspace({ ...baseSession, permissions: ["nodes.manage"], roles: [{ ...baseSession.roles![0], key: "custom" }] })).toBe(true);
  });

  it("allows read-only admin capabilities without letting organization-read-only users into admin", () => {
    expect(canUseAdminWorkspace({ ...baseSession, permissions: ["nodes.read"] })).toBe(true);
    expect(canUseAdminWorkspace({ ...baseSession, permissions: ["rules.read_all"] })).toBe(true);
    expect(canUseAdminWorkspace({ ...baseSession, permissions: ["traffic.read_all"] })).toBe(true);
    expect(canUseAdminWorkspace({ ...baseSession, permissions: ["organization.update"] })).toBe(true);
    expect(canUseAdminWorkspace({ ...baseSession, permissions: ["organization.read"] })).toBe(false);
  });

  it("summarizes multiple roles", () => {
    expect(
      roleSummary({
        ...baseSession,
        roles: [
          { ...baseSession.roles![0], name: "User" },
          { ...baseSession.roles![0], id: "role_02", key: "regional", name: "Regional" },
        ],
      }),
    ).toBe("User, Regional");
  });
});
