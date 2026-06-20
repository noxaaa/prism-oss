import { describe, expect, it } from "vitest";

import { isAuthSignUpPath, isValidOSSSignupSetupToken, resolveSignupPolicy } from "./oss-signup-policy";

describe("OSS signup policy", () => {
  it("allows OSS sign-up before the first organization exists", async () => {
    await expect(resolveSignupPolicy({ database: fakePolicyDatabase(0), webEdition: "oss" })).resolves.toEqual({
      registrationClosed: false,
    });
  });

  it("disables OSS sign-up after the first organization exists", async () => {
    await expect(resolveSignupPolicy({ database: fakePolicyDatabase(1), webEdition: "oss" })).resolves.toEqual({
      registrationClosed: true,
    });
  });

  it("keeps full edition sign-up open after organizations exist", async () => {
    await expect(resolveSignupPolicy({ database: fakePolicyDatabase(1), webEdition: "full" })).resolves.toEqual({
      registrationClosed: false,
    });
  });

  it("matches only email sign-up auth paths", () => {
    expect(isAuthSignUpPath("http://localhost:3000/api/auth/sign-up/email")).toBe(true);
    expect(isAuthSignUpPath("http://localhost:3000/api/auth/sign-in/email")).toBe(false);
    expect(isAuthSignUpPath("http://localhost:3000/api/control/bootstrap")).toBe(false);
  });

  it("requires a valid setup token for first-owner sign-up", () => {
    expect(isValidOSSSignupSetupToken(new Request("http://localhost:3000/api/auth/sign-up/email"), "secret")).toBe(false);
    expect(isValidOSSSignupSetupToken(new Request("http://localhost:3000/api/auth/sign-up/email?setup_token=secret"), "secret")).toBe(true);
    expect(
      isValidOSSSignupSetupToken(
        new Request("http://localhost:3000/api/auth/sign-up/email", {
          headers: { "x-oss-setup-token": "secret" },
        }),
        "secret",
      ),
    ).toBe(true);
    expect(isValidOSSSignupSetupToken(new Request("http://localhost:3000/api/auth/sign-up/email?setup_token=wrong"), "secret")).toBe(false);
    expect(isValidOSSSignupSetupToken(new Request("http://localhost:3000/api/auth/sign-up/email"), "")).toBe(false);
  });
});

function fakePolicyDatabase(count: number) {
  return {
    async query(sql: string) {
      expect(sql).toBe("SELECT count(*) AS count FROM app.organizations WHERE deleted_at IS NULL");
      return { rows: [{ count: String(count) }] };
    },
    async end() {},
  };
}
