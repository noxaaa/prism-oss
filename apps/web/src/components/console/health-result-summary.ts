export type HealthResultSummaryInput = {
  status: string;
  observed_at: string;
  created_at: string;
};

export function summarizeHealthResults<Result extends HealthResultSummaryInput>(results: Result[]): Result | undefined {
  return results.reduce<Result | undefined>((selected, candidate) => {
    if (!selected) return candidate;
    const selectedRank = healthStatusRank(selected.status);
    const candidateRank = healthStatusRank(candidate.status);
    if (candidateRank !== selectedRank) return candidateRank > selectedRank ? candidate : selected;
    return safeTimestamp(candidate.observed_at || candidate.created_at) > safeTimestamp(selected.observed_at || selected.created_at) ? candidate : selected;
  }, undefined);
}

export function countHealthResultsByStatus(results: HealthResultSummaryInput[], status: string): number {
  return results.filter((result) => result.status === status).length;
}

export function formatHealthLatencyMs(latencyMs: number | null | undefined, noneLabel: string): string {
  if (latencyMs == null || latencyMs < 0) {
    return noneLabel;
  }
  return `${latencyMs} ms`;
}

function healthStatusRank(status: string): number {
  const normalized = status.toUpperCase();
  if (normalized === "OFFLINE" || normalized === "FAILED") return 4;
  if (normalized === "DEGRADED" || normalized === "TIMEOUT") return 3;
  if (normalized === "UNKNOWN") return 2;
  if (normalized === "ONLINE" || normalized === "OK") return 1;
  return 0;
}

function safeTimestamp(value: string) {
  const timestamp = Date.parse(value);
  return Number.isNaN(timestamp) ? 0 : timestamp;
}
