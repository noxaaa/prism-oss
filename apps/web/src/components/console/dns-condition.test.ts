import { describe, expect, it } from "vitest";
import { dnsConditionFromPayload, dnsConditionShowsAlwaysMatch, dnsConditionToPayload } from "./features/dns";

describe("DNS condition helpers", () => {
  it("normalizes condition fields accepted by the evaluator", () => {
    const condition = dnsConditionFromPayload({
      op: " or ",
      children: [
        { metric: " Offline_Node_Count ", comparator: " >= ", value: 2 },
      ],
    });

    expect(dnsConditionToPayload(condition)).toEqual({
      op: "OR",
      children: [
        { metric: "offline_node_count", comparator: ">=", value: 2 },
      ],
    });
  });

  it("preserves explicit empty condition groups", () => {
    const condition = dnsConditionFromPayload({ op: "AND", children: [] });

    expect(dnsConditionToPayload(condition)).toEqual({ op: "AND", children: [] });
  });

  it("prunes newly added empty condition groups", () => {
    expect(dnsConditionToPayload({ op: "AND", children: [{ op: "AND", children: [] }] })).toEqual({});
  });

  it("serializes a user-cleared root group as always match", () => {
    const condition = dnsConditionFromPayload({
      op: "AND",
      children: [{ metric: "online_node_count", comparator: ">=", value: 1 }],
    });

    expect(dnsConditionToPayload({ ...condition, children: [], preserveEmpty: false })).toEqual({});
  });

  it("preserves fractional numeric thresholds", () => {
    const condition = dnsConditionFromPayload({
      metric: "online_node_percent",
      comparator: ">=",
      value: 12.5,
    });

    expect(dnsConditionToPayload(condition)).toEqual({
      op: "AND",
      children: [{ metric: "online_node_percent", comparator: ">=", value: 12.5 }],
    });
  });

  it("does not coerce invalid leaf values to zero", () => {
    const raw = {
      metric: "online_node_count",
      comparator: ">=",
      value: "",
    };
    const condition = dnsConditionFromPayload(raw);

    expect(dnsConditionToPayload(condition)).toEqual(raw);
  });

  it("preserves edited invalid leaf values instead of dropping them", () => {
    expect(dnsConditionToPayload({
      op: "AND",
      children: [{ metric: "online_node_count", comparator: ">=", value: "" }],
    })).toEqual({
      op: "AND",
      children: [{ metric: "online_node_count", comparator: ">=", value: "" }],
    });
  });

  it("does not label preserved empty groups as always matching", () => {
    expect(dnsConditionShowsAlwaysMatch(dnsConditionFromPayload({}))).toBe(true);
    expect(dnsConditionShowsAlwaysMatch(dnsConditionFromPayload({ op: "AND", children: [] }))).toBe(false);
  });

  it("preserves unsupported top-level condition payloads", () => {
    const raw = { metric: "future_metric", comparator: ">=", value: 1 };

    expect(dnsConditionToPayload(dnsConditionFromPayload(raw))).toEqual(raw);
  });

  it("preserves unsupported nested condition payloads", () => {
    const raw = { op: "AND", children: [{ metric: "future_metric", comparator: ">=", value: 1 }] };

    expect(dnsConditionToPayload(dnsConditionFromPayload(raw))).toEqual(raw);
  });

  it("preserves malformed nested condition children", () => {
    const raw = {
      op: "AND",
      children: [
        "malformed",
        null,
        {},
        { metric: "online_node_count", comparator: ">=", value: 1 },
      ],
    };

    expect(dnsConditionToPayload(dnsConditionFromPayload(raw))).toEqual(raw);
  });
});
