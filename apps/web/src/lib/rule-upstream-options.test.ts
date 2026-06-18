import { describe, expect, it } from "vitest";
import { buildRuleUpstreamOptionPaths } from "./rule-upstream-options";

describe("buildRuleUpstreamOptionPaths", () => {
  it("requests upstream options without target-owned protocol filtering", () => {
    expect(buildRuleUpstreamOptionPaths()).toEqual({
      targets: "/api/control/resource-options/targets",
      targetGroups: "/api/control/resource-options/target-groups",
    });
  });
});
