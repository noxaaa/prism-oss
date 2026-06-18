import { describe, expect, it } from "vitest";

import { ControlAPIError } from "./control-api";

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
});
