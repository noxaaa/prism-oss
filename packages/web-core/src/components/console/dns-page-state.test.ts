import { describe, expect, it } from "vitest";
import { canUseDNSHealthSelector, dnsPageResourceState } from "./dns-page-state";
import type { ControlSession } from "./types";

describe("dns page state", () => {
  const dnsOnlySession: ControlSession = {
    user: { id: "user_1", name: "DNS Operator", email: "dns@example.com" },
    organization: { id: "org_1", name: "Org", slug: "org" },
    permissions: ["dns.read", "dns.manage"],
    roles: [],
  };

  it("does not require health-check permissions for the DNS page", () => {
    expect(canUseDNSHealthSelector(dnsOnlySession)).toBe(false);

    const state = dnsPageResourceState({
      credentials: { loading: false, error: "" },
      records: { loading: false, error: "" },
      healthChecks: { loading: false, error: "Forbidden" },
      includeHealthChecks: false,
    });

    expect(state).toEqual({ loading: false, error: "" });
  });

  it("includes health-check state when the selector is available", () => {
    const state = dnsPageResourceState({
      credentials: { loading: false, error: "" },
      records: { loading: false, error: "" },
      healthChecks: { loading: false, error: "Forbidden" },
      includeHealthChecks: true,
    });

    expect(state).toEqual({ loading: false, error: "Forbidden" });
  });
});
