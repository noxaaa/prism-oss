import { describe, expect, it } from "vitest";
import { callControlSession, proxyControlRequest } from "./control-client";

describe("control client", () => {
  it("calls internal session with a session-purpose web user token", async () => {
    const calls: Request[] = [];
    const responseBody = { data: { user: { id: "user_1", email: "owner@example.com", name: "Owner" }, organizations: [] } };
    const fetcher = async (request: Request) => {
      calls.push(request);
      return Response.json(responseBody);
    };

    const response = await callControlSession({
      controlPlaneURL: "http://control.local",
      internalSecret: "test-secret",
      user: { id: "user_1", email: "owner@example.com", name: "Owner" },
      fetcher,
    });

    expect(response).toEqual(responseBody);
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("http://control.local/internal/v1/session");
    const payload = decodePayload(calls[0].headers.get("authorization"));
    expect(payload).toMatchObject({ user_id: "user_1", purpose: "session", source_service: "web" });
  });

  it("proxies org-scoped control requests with permissions and resource scopes from the session", async () => {
    const calls: Request[] = [];
    const fetcher = async (request: Request) => {
      calls.push(request);
      if (request.url.endsWith("/internal/v1/session")) {
        return Response.json({
          data: {
            user: { id: "user_1", email: "owner@example.com", name: "Owner" },
            organization: { id: "org_1", name: "Org", slug: "org" },
            member: { id: "member_1", user_id: "user_1", email: "owner@example.com", name: "Owner", status: "ACTIVE" },
            roles: [{ key: "owner" }],
            permissions: ["organization.read"],
            resource_scopes: [{ resource_type: "NODE_GROUP", resource_id: "*", access_level: "MANAGE" }],
          },
        });
      }
      return Response.json({ data: { ok: true } });
    };

    const response = await proxyControlRequest({
      controlPlaneURL: "http://control.local",
      internalSecret: "test-secret",
      user: { id: "user_1", email: "owner@example.com", name: "Owner" },
      path: "/internal/v1/organizations/current",
      init: { method: "GET" },
      fetcher,
    });

    expect(await response.json()).toEqual({ data: { ok: true } });
    expect(calls).toHaveLength(2);
    const payload = decodePayload(calls[1].headers.get("authorization"));
    expect(payload).toMatchObject({
      user_id: "user_1",
      organization_id: "org_1",
      member_id: "member_1",
      roles: ["owner"],
      permissions: ["organization.read"],
      resource_scopes: [{ resource_type: "NODE_GROUP", resource_id: "*", access_level: "MANAGE" }],
    });
  });

  it("preserves failed session responses instead of signing an org-scoped token", async () => {
    const calls: Request[] = [];
    const fetcher = async (request: Request) => {
      calls.push(request);
      return Response.json({ error: { code: "UNAUTHENTICATED" } }, { status: 401 });
    };

    await expect(
      proxyControlRequest({
        controlPlaneURL: "http://control.local",
        internalSecret: "test-secret",
        user: { id: "user_1", email: "owner@example.com", name: "Owner" },
        path: "/internal/v1/organizations/current",
        init: { method: "GET" },
        fetcher,
      }),
    ).rejects.toMatchObject({ status: 401, code: "UNAUTHENTICATED" });
    expect(calls).toHaveLength(1);
  });
});

function decodePayload(authorization: string | null): Record<string, unknown> {
  expect(authorization).toMatch(/^Bearer /);
  const token = authorization?.slice("Bearer ".length) ?? "";
  return JSON.parse(Buffer.from(token.split(".")[0], "base64url").toString("utf8")) as Record<string, unknown>;
}
