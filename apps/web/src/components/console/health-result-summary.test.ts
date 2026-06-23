import { describe, expect, it } from "vitest";

import { countHealthResultsByStatus, formatHealthLatencyMs, summarizeHealthResults } from "./health-result-summary";

describe("health result summary", () => {
  it("prefers unhealthy statuses over newer healthy results", () => {
    const summary = summarizeHealthResults([
      { status: "OFFLINE", observed_at: "2026-06-20T00:00:00Z", created_at: "2026-06-20T00:00:00Z" },
      { status: "ONLINE", observed_at: "2026-06-20T00:01:00Z", created_at: "2026-06-20T00:01:00Z" },
    ]);

    expect(summary?.status).toBe("OFFLINE");
  });

  it("counts matching statuses for aggregated health labels", () => {
    expect(countHealthResultsByStatus([
      { status: "OFFLINE", observed_at: "", created_at: "" },
      { status: "ONLINE", observed_at: "", created_at: "" },
      { status: "OFFLINE", observed_at: "", created_at: "" },
    ], "OFFLINE")).toBe(2);
  });

  it("formats missing latency sentinel values as none", () => {
    expect(formatHealthLatencyMs(null, "None")).toBe("None");
    expect(formatHealthLatencyMs(-1, "None")).toBe("None");
    expect(formatHealthLatencyMs(42, "None")).toBe("42 ms");
  });
});
