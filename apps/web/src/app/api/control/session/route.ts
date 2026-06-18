import { callControlSessionResponse } from "@/lib/control-client";
import {
  errorResponse,
  getAuthenticatedControlUser,
  getControlBFFConfig,
  handleControlBFFError,
} from "@/lib/control-bff";

export const runtime = "nodejs";

export async function GET(request: Request): Promise<Response> {
  const user = await getAuthenticatedControlUser(request.headers);
  if (!user) {
    return errorResponse(401, "UNAUTHENTICATED", "Sign in before requesting a control session");
  }

  try {
    const config = getControlBFFConfig();
    return await callControlSessionResponse({
      ...config,
      user,
    });
  } catch (error) {
    return handleControlBFFError(error);
  }
}
