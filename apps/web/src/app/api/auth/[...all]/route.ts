import { toNextJsHandler } from "better-auth/next-js";
import { auth } from "@/lib/auth";
import { isAuthSignUpPath, resolveSignupPolicy } from "@/lib/oss-signup-policy";

export const runtime = "nodejs";

const authHandlers = toNextJsHandler(auth);

export const GET = authHandlers.GET;

export async function POST(request: Request): Promise<Response> {
  if (isAuthSignUpPath(request.url) && resolveSignupPolicy().registrationClosed) {
    return Response.json(
      {
        error: {
          code: "OSS_SIGNUP_DISABLED",
          message: "OSS registration is closed.",
          details: { edition: "oss" },
        },
      },
      { status: 403 },
    );
  }
  return authHandlers.POST(request);
}
