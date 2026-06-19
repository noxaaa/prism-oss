import { describe, expect, it } from "vitest";
import {
  TARGET_GROUP_SCHEDULER_PRIORITY_IPHASH,
  buildTargetGroupMutationPayload,
  membersForSelectedTargets,
  type TargetGroupMemberDraft,
} from "./target-group-members";

describe("target group member helpers", () => {
  it("preserves each selected target priority and enabled state in mutation payloads", () => {
    const members: TargetGroupMemberDraft[] = [
      { target_id: "target_a", priority: 10, enabled: true },
      { target_id: "target_b", priority: 20, enabled: false },
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
        { target_id: "target_a", priority: 10, enabled: true },
        { target_id: "target_b", priority: 20, enabled: false },
      ],
    });
  });

  it("adds newly selected targets with default priority without losing existing members", () => {
    const next = membersForSelectedTargets(
      ["target_b", "target_a"],
      [{ target_id: "target_a", priority: 30, enabled: false }],
    );

    expect(next).toEqual([
      { target_id: "target_b", priority: 10, enabled: true },
      { target_id: "target_a", priority: 30, enabled: false },
    ]);
  });
});
