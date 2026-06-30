import { describe, expect, it } from "vitest";

import { ControlAPIError, formatBitrateBps } from "./control-api";

describe("ControlAPIError", () => {
  it("keeps stable code and details for frontend localization", () => {
    const error = new ControlAPIError(400, "VALIDATION_FAILED", "Import format is required.", {
      field: "format",
      supported_formats: ["PORTABLE_EXPORT", "NYANPASS"],
    });

    expect(error.status).toBe(400);
    expect(error.code).toBe("VALIDATION_FAILED");
    expect(error.message).toBe("Import format is required.");
    expect(error.details).toEqual({
      field: "format",
      supported_formats: ["PORTABLE_EXPORT", "NYANPASS"],
    });
  });

  it("formats bit rates with decimal network units", () => {
    expect(formatBitrateBps(0)).toBe("0 bps");
    expect(formatBitrateBps(999)).toBe("999 bps");
    expect(formatBitrateBps(2329137)).toBe("2.3 Mbps");
    expect(formatBitrateBps(1_250_000_000)).toBe("1.3 Gbps");
  });
});
