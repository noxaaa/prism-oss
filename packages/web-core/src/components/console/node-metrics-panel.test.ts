import { describe, expect, it } from "vitest";
import { missingMetricsLabel } from "./features/node-metrics-panel";

const t = (key: string) => ({
  "nodes.metricsWaiting": "Waiting for metrics",
  "status.OFFLINE": "Offline",
  "status.PENDING": "Pending",
  "status.DISABLED": "Disabled",
}[key] ?? key);

describe("node metrics empty labels", () => {
  it("shows waiting for online nodes before metrics arrive", () => {
    expect(missingMetricsLabel({ status: "ONLINE" }, t)).toBe("Waiting for metrics");
  });

  it("preserves pending node status before metrics arrive", () => {
    expect(missingMetricsLabel({ status: "PENDING" }, t)).toBe("Pending");
  });

  it("preserves offline node status before metrics arrive", () => {
    expect(missingMetricsLabel({ status: "OFFLINE" }, t)).toBe("Offline");
  });
});
