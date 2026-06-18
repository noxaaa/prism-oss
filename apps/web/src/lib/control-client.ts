import { signInternalIdentityToken, signWebUserToken, type ResourceScopeClaim } from "./control-token";

export type ControlUser = {
  id: string;
  email: string;
  name?: string | null;
};

export type ControlClientInput = {
  controlPlaneURL: string;
  internalSecret: string;
  user: ControlUser;
  fetcher?: (request: Request) => Promise<Response>;
};

export type ControlSessionResponse = {
  data: {
    user: ControlUser;
    organizations?: Array<{ id: string; name: string; slug: string }>;
    organization?: { id: string; name: string; slug: string };
    member?: { id: string; user_id: string; email: string; name?: string | null; status: string };
    roles?: Array<{ key: string } | string>;
    permissions?: string[];
    resource_scopes?: ResourceScopeClaim[];
  };
};

export class ControlClientError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string) {
    super(code);
    this.status = status;
    this.code = code;
  }
}

export async function callControlSession(input: ControlClientInput): Promise<ControlSessionResponse> {
  const response = await callControlSessionResponse(input);
  const body = (await response.json()) as ControlSessionResponse & { error?: { code?: string } };
  if (!response.ok) {
    throw new ControlClientError(response.status, body.error?.code ?? "CONTROL_API_ERROR");
  }
  return body;
}

export async function callControlSessionResponse(input: ControlClientInput): Promise<Response> {
  return callControlWithWebUserToken({
    ...input,
    path: "/internal/v1/session",
    purpose: "session",
    init: { method: "GET" },
  });
}

export async function callControlBootstrap(input: ControlClientInput & { init: RequestInit }): Promise<Response> {
  return callControlWithWebUserToken({
    ...input,
    path: "/internal/v1/bootstrap",
    purpose: "bootstrap",
    init: input.init,
  });
}

export async function proxyControlRequest(
  input: ControlClientInput & {
    path: string;
    init: RequestInit;
  },
): Promise<Response> {
  const session = await callControlSession(input);
  const organizationID = session.data.organization?.id;
  const memberID = session.data.member?.id;
  if (!organizationID || !memberID) {
    throw new Error("Control session is not associated with an organization member");
  }

  const token = signInternalIdentityToken({
    secret: input.internalSecret,
    identity: {
      userId: input.user.id,
      organizationId: organizationID,
      memberId: memberID,
      roles: normalizeRoleKeys(session.data.roles ?? []),
      permissions: session.data.permissions ?? [],
      resourceScopes: session.data.resource_scopes ?? [],
    },
    expiresAt: tokenExpiry(),
  });
  return fetchWithBearer({
    controlPlaneURL: input.controlPlaneURL,
    fetcher: input.fetcher,
    path: input.path,
    token,
    init: input.init,
  });
}

async function callControlWithWebUserToken(
  input: ControlClientInput & {
    path: string;
    purpose: "bootstrap" | "session";
    init: RequestInit;
  },
): Promise<Response> {
  const token = signWebUserToken({
    secret: input.internalSecret,
    user: input.user,
    purpose: input.purpose,
    expiresAt: tokenExpiry(),
  });
  return fetchWithBearer({
    controlPlaneURL: input.controlPlaneURL,
    fetcher: input.fetcher,
    path: input.path,
    token,
    init: input.init,
  });
}

function fetchWithBearer(input: {
  controlPlaneURL: string;
  fetcher?: (request: Request) => Promise<Response>;
  path: string;
  token: string;
  init: RequestInit;
}): Promise<Response> {
  const headers = new Headers(input.init.headers);
  headers.set("authorization", `Bearer ${input.token}`);
  const request = new Request(joinURL(input.controlPlaneURL, input.path), {
    ...input.init,
    headers,
  });
  return (input.fetcher ?? fetch)(request);
}

function normalizeRoleKeys(roles: Array<{ key: string } | string>): string[] {
  return roles.map((role) => (typeof role === "string" ? role : role.key));
}

function joinURL(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}/${path.replace(/^\/+/, "")}`;
}

function tokenExpiry(): Date {
  return new Date(Date.now() + 5 * 60 * 1000);
}
