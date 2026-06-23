import { describe, expect, it } from "vitest";

import { formatHealthCheckTargets } from "./health-check-targets";

describe("health check target display helpers", () => {
  it("does not render target-group placeholder bindings as blank targets", () => {
    const value = formatHealthCheckTargets(
      [
        { id: "placeholder", scope_type: "TARGET_GROUP", target_group_id: "group_1", target_id: "", target_name: "", target_host: "", target_port: 0 },
        { id: "target_1", scope_type: "TARGET_GROUP", target_group_id: "group_1", target_id: "target_1", target_name: "api", target_host: "192.0.2.1", target_port: 443 },
      ],
      [{ id: "group_1", name: "primary", description: "", scheduler: "PRIORITY_IPHASH", members: [] }],
    );

    expect(value).toBe("api (192.0.2.1:443)");
    expect(value).not.toContain("(:0)");
  });

  it("shows the target group name when an empty group only has a placeholder binding", () => {
    expect(formatHealthCheckTargets(
      [{ id: "placeholder", scope_type: "TARGET_GROUP", target_group_id: "group_1", target_id: "", target_name: "", target_host: "", target_port: 0 }],
      [{ id: "group_1", name: "empty group", description: "", scheduler: "PRIORITY_IPHASH", members: [] }],
    )).toBe("empty group");
  });

  it("falls back to target_scope when the service omits empty target group bindings", () => {
    expect(formatHealthCheckTargets(
      [],
      [{ id: "group_1", name: "empty group", description: "", scheduler: "PRIORITY_IPHASH", members: [] }],
      { type: "TARGET_GROUP", target_group_id: "group_1" },
    )).toBe("empty group");
  });
});
