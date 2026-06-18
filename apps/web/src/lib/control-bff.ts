import { auth } from "./auth";
import { ControlClientError } from "./control-client";
import type { ControlUser } from "./control-client";

export type ControlBFFConfig = {
  controlPlaneURL: string;
  internalSecret: string;
};

export async function getAuthenticatedControlUser(headers: Headers): Promise<ControlUser | null> {
  const session = await auth.api.getSession({ headers });
  if (!session?.user.id || !session.user.email) {
    return null;
  }
  return {
    id: session.user.id,
    email: session.user.email,
    name: session.user.name ?? "",
  };
}

export function getControlBFFConfig(): ControlBFFConfig {
  const controlPlaneURL = process.env.CONTROL_PLANE_INTERNAL_URL;
  const internalSecret = process.env.CONTROL_PLANE_INTERNAL_JWT_SECRET;
  if (!controlPlaneURL || !internalSecret) {
    throw new Error("CONTROL_PLANE_INTERNAL_URL and CONTROL_PLANE_INTERNAL_JWT_SECRET are required");
  }
  return { controlPlaneURL, internalSecret };
}

export function errorResponse(status: number, code: string, message: string): Response {
  return Response.json({ error: { code, message } }, { status });
}

export function forwardableHeaders(headers: Headers): Headers {
  const forwarded = new Headers();
  const contentType = headers.get("content-type");
  const accept = headers.get("accept");
  if (contentType) {
    forwarded.set("content-type", contentType);
  }
  if (accept) {
    forwarded.set("accept", accept);
  }
  return forwarded;
}

export function handleControlBFFError(error: unknown): Response {
  if (error instanceof ControlClientError) {
    return errorResponse(error.status, error.code, error.message);
  }
  const message = error instanceof Error ? error.message : "Control API proxy failed";
  if (message.includes("not associated with an organization member")) {
    return errorResponse(403, "FORBIDDEN", message);
  }
  if (message.includes("CONTROL_PLANE_INTERNAL_URL")) {
    return errorResponse(500, "CONFIGURATION_ERROR", message);
  }
  return errorResponse(502, "CONTROL_PLANE_UNAVAILABLE", message);
}
