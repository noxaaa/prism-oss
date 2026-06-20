import { toNextJsHandler } from "better-auth/next-js";
import { auth } from "@/lib/auth";
import { isAuthSignUpPath, isValidOSSSignupSetupToken, resolveSignupPolicy, webEditionFromEnv } from "@/lib/oss-signup-policy";

export const runtime = "nodejs";

const authHandlers = toNextJsHandler(auth);

export const GET = authHandlers.GET;

export async function POST(request: Request): Promise<Response> {
  if (isAuthSignUpPath(request.url)) {
    if ((await resolveSignupPolicy()).registrationClosed) {
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
    if (webEditionFromEnv() === "oss" && !isValidOSSSignupSetupToken(request)) {
      return Response.json(
        {
          error: {
            code: "OSS_SETUP_TOKEN_REQUIRED",
            message: "A valid OSS setup token is required.",
            details: { edition: "oss" },
          },
        },
        { status: 403 },
      );
    }
  }
  return authHandlers.POST(request);
}
