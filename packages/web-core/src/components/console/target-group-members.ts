export const TARGET_GROUP_SCHEDULER_PRIORITY_IPHASH = "PRIORITY_IPHASH";
export const TARGET_GROUP_SCHEDULER_LEAST_LOAD = "LEAST_LOAD";
export const DEFAULT_TARGET_GROUP_MEMBER_PRIORITY = 10;
export const DEFAULT_TARGET_GROUP_MEMBER_WEIGHT = 1;
export const MAX_TARGET_GROUP_MEMBER_WEIGHT = 256;

export type TargetGroupMemberDraft = {
  target_id: string;
  priority: number;
  weight: number;
  enabled: boolean;
};

export type TargetGroupMutationPayload = {
  name: string;
  description: string;
  scheduler: string;
  members: TargetGroupMemberDraft[];
};

export function membersForSelectedTargets(
  targetIDs: string[],
  existingMembers: TargetGroupMemberDraft[],
  defaultPriority = DEFAULT_TARGET_GROUP_MEMBER_PRIORITY,
): TargetGroupMemberDraft[] {
  const existingByTargetID = new Map(existingMembers.map((member) => [member.target_id, member]));
  return targetIDs.map((targetID) => {
    const existing = existingByTargetID.get(targetID);
    if (existing) {
      return { ...existing };
    }
    return { target_id: targetID, priority: defaultPriority, weight: DEFAULT_TARGET_GROUP_MEMBER_WEIGHT, enabled: true };
  });
}

export function buildTargetGroupMutationPayload(input: {
  name: FormDataEntryValue | string | null;
  description: FormDataEntryValue | string | null;
  scheduler?: string;
  members: TargetGroupMemberDraft[];
}): TargetGroupMutationPayload {
  return {
    name: formValueToString(input.name),
    description: formValueToString(input.description),
    scheduler: normalizeTargetGroupScheduler(input.scheduler),
    members: input.members.map((member) => ({
      target_id: member.target_id,
      priority: normalizePriority(member.priority),
      weight: normalizeWeight(member.weight),
      enabled: member.enabled,
    })),
  };
}

export function normalizePriority(priority: number): number {
  if (!Number.isFinite(priority)) {
    return DEFAULT_TARGET_GROUP_MEMBER_PRIORITY;
  }
  return Math.max(0, Math.trunc(priority));
}

export function normalizeWeight(weight: number): number {
  if (!Number.isFinite(weight)) {
    return DEFAULT_TARGET_GROUP_MEMBER_WEIGHT;
  }
  return Math.min(MAX_TARGET_GROUP_MEMBER_WEIGHT, Math.max(0, Math.trunc(weight)));
}

export function normalizeTargetGroupScheduler(scheduler?: string): string {
  const normalized = scheduler?.trim().toUpperCase();
  return normalized || TARGET_GROUP_SCHEDULER_PRIORITY_IPHASH;
}

function formValueToString(value: FormDataEntryValue | string | null): string {
  if (typeof value === "string") {
    return value;
  }
  return "";
}
