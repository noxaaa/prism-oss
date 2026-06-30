import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

describe("target group editor source", () => {
  it("uses per-member target group editing and submits the scheduler explicitly", () => {
    const source = readFileSync(join(process.cwd(), "src/components/console/features/targets.tsx"), "utf8");

    expect(source).toContain("TargetGroupMembersEditor");
    expect(source).toContain("buildTargetGroupMutationPayload");
	    expect(source).toContain("TARGET_GROUP_SCHEDULER_LEAST_LOAD");
	    expect(source).toContain("!isKnownScheduler ? <SelectItem value={normalizedValue}>{normalizedValue}</SelectItem> : null");
	    expect(source).toContain("max={MAX_TARGET_GROUP_MEMBER_WEIGHT}");
	    expect(source).toContain("scheduler,");
    expect(source).toContain("targets.memberWeight");
    expect(source).not.toContain("targets.defaultMemberPriority");
  });
});
