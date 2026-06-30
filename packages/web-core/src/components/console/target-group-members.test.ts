import { describe, expect, it } from "vitest";
import {
  TARGET_GROUP_SCHEDULER_LEAST_LOAD,
  TARGET_GROUP_SCHEDULER_PRIORITY_IPHASH,
  buildTargetGroupMutationPayload,
  membersForSelectedTargets,
  type TargetGroupMemberDraft,
} from "./target-group-members";

describe("target group member helpers", () => {
  it("preserves each selected target priority and enabled state in mutation payloads", () => {
    const members: TargetGroupMemberDraft[] = [
      { target_id: "target_a", priority: 10, weight: 2, enabled: true },
      { target_id: "target_b", priority: 20, weight: 0, enabled: false },
    ];

    const payload = buildTargetGroupMutationPayload({
      name: "Pool",
      description: "Priority pool",
      scheduler: TARGET_GROUP_SCHEDULER_PRIORITY_IPHASH,
      members,
    });

    expect(payload).toEqual({
      name: "Pool",
      description: "Priority pool",
      scheduler: "PRIORITY_IPHASH",
      members: [
        { target_id: "target_a", priority: 10, weight: 2, enabled: true },
        { target_id: "target_b", priority: 20, weight: 0, enabled: false },
      ],
    });
  });

  it("normalizes least-load scheduler and default member weight", () => {
    const payload = buildTargetGroupMutationPayload({
      name: "Pool",
      description: "Least load pool",
      scheduler: " least_load ",
      members: [{ target_id: "target_a", priority: 10, weight: Number.NaN, enabled: true }],
    });

	  expect(payload.scheduler).toBe(TARGET_GROUP_SCHEDULER_LEAST_LOAD);
	  expect(payload.members[0].weight).toBe(1);
	});

	it("clamps member weight to HAProxy's supported maximum", () => {
		const payload = buildTargetGroupMutationPayload({
			name: "Pool",
			description: "Least load pool",
			scheduler: TARGET_GROUP_SCHEDULER_LEAST_LOAD,
			members: [{ target_id: "target_a", priority: 10, weight: 257, enabled: true }],
		});

		expect(payload.members[0].weight).toBe(256);
	});

	it("adds newly selected targets with default priority without losing existing members", () => {
    const next = membersForSelectedTargets(
      ["target_b", "target_a"],
      [{ target_id: "target_a", priority: 30, weight: 2, enabled: false }],
    );

    expect(next).toEqual([
      { target_id: "target_b", priority: 10, weight: 1, enabled: true },
      { target_id: "target_a", priority: 30, weight: 2, enabled: false },
    ]);
  });
});
