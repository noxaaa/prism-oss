import { callControlBootstrap } from "@/lib/control-client";
import {
  errorResponse,
  forwardableHeaders,
  getAuthenticatedControlUser,
  getControlBFFConfig,
  handleControlBFFError,
} from "@/lib/control-bff";

export const runtime = "nodejs";

export async function POST(request: Request): Promise<Response> {
  const user = await getAuthenticatedControlUser(request.headers);
  if (!user) {
    return errorResponse(401, "UNAUTHENTICATED", "Sign in before bootstrapping an organization");
  }

  try {
    const config = getControlBFFConfig();
    return await callControlBootstrap({
      ...config,
      user,
      init: {
        method: "POST",
        headers: forwardableHeaders(request.headers),
        body: await request.text(),
      },
    });
  } catch (error) {
    return handleControlBFFError(error);
  }
}
