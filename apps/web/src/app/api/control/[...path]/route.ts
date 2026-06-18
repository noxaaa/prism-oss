import { proxyControlRequest } from "@/lib/control-client";
import {
  errorResponse,
  forwardableHeaders,
  getAuthenticatedControlUser,
  getControlBFFConfig,
  handleControlBFFError,
} from "@/lib/control-bff";

export const runtime = "nodejs";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

export async function GET(request: Request, context: RouteContext): Promise<Response> {
  return proxy(request, context);
}

export async function POST(request: Request, context: RouteContext): Promise<Response> {
  return proxy(request, context);
}

export async function PATCH(request: Request, context: RouteContext): Promise<Response> {
  return proxy(request, context);
}

export async function DELETE(request: Request, context: RouteContext): Promise<Response> {
  return proxy(request, context);
}

async function proxy(request: Request, context: RouteContext): Promise<Response> {
  const user = await getAuthenticatedControlUser(request.headers);
  if (!user) {
    return errorResponse(401, "UNAUTHENTICATED", "Sign in before calling the control API");
  }

  try {
    const config = getControlBFFConfig();
    const params = await context.params;
    const url = new URL(request.url);
    const path = `/internal/v1/${params.path.map(encodeURIComponent).join("/")}${url.search}`;
    return await proxyControlRequest({
      ...config,
      user,
      path,
      init: {
        method: request.method,
        headers: forwardableHeaders(request.headers),
        body: request.method === "GET" || request.method === "HEAD" ? undefined : await request.text(),
      },
    });
  } catch (error) {
    return handleControlBFFError(error);
  }
}
