import { createHmac } from "node:crypto";

export type WebUserTokenPurpose = "bootstrap" | "session";

export type WebUserTokenInput = {
  secret: string;
  user: {
    id: string;
    email: string;
    name?: string | null;
  };
  purpose: WebUserTokenPurpose;
  expiresAt: Date;
};

export type ResourceScopeClaim = {
  resource_type: string;
  resource_id: string;
  access_level: string;
};

export type InternalIdentityTokenInput = {
  secret: string;
  identity: {
    userId: string;
    organizationId: string;
    memberId: string;
    roles: string[];
    permissions: string[];
    resourceScopes: ResourceScopeClaim[];
  };
  expiresAt: Date;
};

export function signWebUserToken(input: WebUserTokenInput): string {
  return signJSON(input.secret, {
    user_id: input.user.id,
    email: input.user.email,
    name: input.user.name ?? "",
    source_service: "web",
    purpose: input.purpose,
    expires_at: input.expiresAt.toISOString(),
  });
}

export function signInternalIdentityToken(input: InternalIdentityTokenInput): string {
  return signJSON(input.secret, {
    user_id: input.identity.userId,
    organization_id: input.identity.organizationId,
    member_id: input.identity.memberId,
    source_service: "web",
    roles: input.identity.roles,
    permissions: input.identity.permissions,
    resource_scopes: input.identity.resourceScopes,
    expires_at: input.expiresAt.toISOString(),
  });
}

function signJSON(secret: string, payload: Record<string, unknown>): string {
  const encodedPayload = Buffer.from(JSON.stringify(payload)).toString("base64url");
  const signature = createHmac("sha256", secret).update(Buffer.from(encodedPayload, "base64url")).digest("base64url");
  return `${encodedPayload}.${signature}`;
}
