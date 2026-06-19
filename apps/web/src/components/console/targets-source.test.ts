import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

describe("target group editor source", () => {
  it("uses per-member target group editing and submits the scheduler explicitly", () => {
    const source = readFileSync(join(process.cwd(), "src/components/console/features/targets.tsx"), "utf8");

    expect(source).toContain("TargetGroupMembersEditor");
    expect(source).toContain("buildTargetGroupMutationPayload");
    expect(source).toContain("scheduler: TARGET_GROUP_SCHEDULER_PRIORITY_IPHASH");
    expect(source).toContain("scheduler: targetGroup.scheduler");
    expect(source).not.toContain("targets.defaultMemberPriority");
  });
});
