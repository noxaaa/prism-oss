import { createHmac } from "node:crypto";
import { describe, expect, it } from "vitest";
import { signInternalIdentityToken, signWebUserToken } from "./control-token";

describe("control token signing", () => {
  it("signs web user tokens with a purpose and web source", () => {
    const token = signWebUserToken({
      secret: "test-secret",
      user: { id: "user_1", email: "owner@example.com", name: "Owner" },
      purpose: "bootstrap",
      expiresAt: new Date("2026-01-01T00:05:00.000Z"),
    });

    const payload = verifiedPayload(token, "test-secret");

    expect(payload).toMatchObject({
      user_id: "user_1",
      email: "owner@example.com",
      name: "Owner",
      source_service: "web",
      purpose: "bootstrap",
      expires_at: "2026-01-01T00:05:00.000Z",
    });
  });

  it("signs org-scoped identity tokens with permissions and resource scopes", () => {
    const token = signInternalIdentityToken({
      secret: "test-secret",
      identity: {
        userId: "user_1",
        organizationId: "org_1",
        memberId: "member_1",
        roles: ["owner"],
        permissions: ["rules.manage_own"],
        resourceScopes: [{ resource_type: "NODE_GROUP", resource_id: "*", access_level: "MANAGE" }],
      },
      expiresAt: new Date("2026-01-01T00:05:00.000Z"),
    });

    const payload = verifiedPayload(token, "test-secret");

    expect(payload).toMatchObject({
      user_id: "user_1",
      organization_id: "org_1",
      member_id: "member_1",
      source_service: "web",
      roles: ["owner"],
      permissions: ["rules.manage_own"],
      resource_scopes: [{ resource_type: "NODE_GROUP", resource_id: "*", access_level: "MANAGE" }],
      expires_at: "2026-01-01T00:05:00.000Z",
    });
  });
});

function verifiedPayload(token: string, secret: string): Record<string, unknown> {
  const [encodedPayload, encodedSignature] = token.split(".");
  const expectedSignature = createHmac("sha256", secret).update(Buffer.from(encodedPayload, "base64url")).digest("base64url");
  expect(encodedSignature).toBe(expectedSignature);
  return JSON.parse(Buffer.from(encodedPayload, "base64url").toString("utf8")) as Record<string, unknown>;
}
