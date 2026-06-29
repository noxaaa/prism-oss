import { describe, expect, it } from "vitest";
import { applyNodeEnrollmentExpiryPayload } from "./features/node-enrollment-profiles";

describe("node enrollment profile expiry payload", () => {
  it("clears an existing expiry when an edited profile is set to never expire", () => {
    const payload: Record<string, unknown> = {};

    applyNodeEnrollmentExpiryPayload(payload, {
      editing: true,
      initialExpiresAt: "2026-07-28T00:00:00Z",
      initialTTLHours: "720",
      neverExpires: true,
      submittedTTLHours: "720",
    });

    expect(payload).toEqual({ expires_at: "" });
  });

  it("sends ttl_hours when re-enabling expiry on a never-expiring profile", () => {
    const payload: Record<string, unknown> = {};

    applyNodeEnrollmentExpiryPayload(payload, {
      editing: true,
      initialExpiresAt: "",
      initialTTLHours: "720",
      neverExpires: false,
      submittedTTLHours: "720",
    });

    expect(payload).toEqual({ ttl_hours: 720 });
  });

  it("keeps an unchanged existing expiry without recalculating ttl", () => {
    const payload: Record<string, unknown> = {};

    applyNodeEnrollmentExpiryPayload(payload, {
      editing: true,
      initialExpiresAt: "2026-07-28T00:00:00Z",
      initialTTLHours: "720",
      neverExpires: false,
      submittedTTLHours: "720",
    });

    expect(payload).toEqual({ expires_at: "2026-07-28T00:00:00Z" });
  });

  it("omits expiry fields for new never-expiring profiles", () => {
    const payload: Record<string, unknown> = {};

    applyNodeEnrollmentExpiryPayload(payload, {
      editing: false,
      initialExpiresAt: "",
      initialTTLHours: "720",
      neverExpires: true,
      submittedTTLHours: "720",
    });

    expect(payload).toEqual({});
  });
});
