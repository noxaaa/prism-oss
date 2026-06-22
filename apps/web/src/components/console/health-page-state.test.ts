import { describe, expect, it } from "vitest";
import { canReadHealthChecks, canUseHealthCheckEditor, healthPageResourceState } from "./health-page-state";
import type { ControlSession } from "./types";

function sessionWith(permissions: string[]): ControlSession {
  return {
    user: { id: "user_1", name: "Operator", email: "operator@example.com" },
    organization: { id: "org_1", name: "Org" },
    permissions,
    roles: [],
  };
}

describe("health page state", () => {
  it("requires read permission to expose the health page", () => {
    expect(canReadHealthChecks(sessionWith(["health_checks.manage"]))).toBe(false);
    expect(canReadHealthChecks(sessionWith(["health_checks.read"]))).toBe(true);
  });

  it("requires dependent target and monitor permissions before loading create selectors", () => {
    expect(canUseHealthCheckEditor(sessionWith(["health_checks.read", "health_checks.manage"]))).toBe(false);
    expect(canUseHealthCheckEditor(sessionWith(["health_checks.read", "health_checks.manage", "targets.read", "monitors.manage"]))).toBe(false);
    expect(canUseHealthCheckEditor(sessionWith(["health_checks.read", "health_checks.manage", "targets.read", "monitors.read"]))).toBe(true);
  });

  it("ignores selector dependency errors when the editor is unavailable", () => {
    const state = healthPageResourceState({
      checks: { loading: false, error: "" },
      targets: { loading: false, error: "Forbidden" },
      targetGroups: { loading: false, error: "Forbidden" },
      monitors: { loading: false, error: "Forbidden" },
      monitorGroups: { loading: false, error: "Forbidden" },
      includeEditorDependencies: false,
    });

    expect(state).toEqual({ loading: false, error: "" });
  });

  it("includes selector dependency errors when the editor is available", () => {
    const state = healthPageResourceState({
      checks: { loading: false, error: "" },
      targets: { loading: false, error: "" },
      targetGroups: { loading: false, error: "Forbidden" },
      monitors: { loading: false, error: "" },
      monitorGroups: { loading: false, error: "" },
      includeEditorDependencies: true,
    });

    expect(state).toEqual({ loading: false, error: "Forbidden" });
  });
});
