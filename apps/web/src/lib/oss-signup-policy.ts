import type { Database as SQLiteDatabase } from "better-sqlite3";

import { createSQLiteDatabase } from "./auth";

export type WebEdition = "oss" | "full";

export type SignupPolicy = {
  registrationClosed: boolean;
};

type SignupPolicyOptions = {
  database?: SQLiteDatabase;
  webEdition?: WebEdition;
};

export function isAuthSignUpPath(url: string): boolean {
  return new URL(url).pathname === "/api/auth/sign-up/email";
}

export function isValidOSSSignupSetupToken(request: Request, expectedToken = process.env.OSS_SETUP_TOKEN): boolean {
  if (!expectedToken) {
    return false;
  }
  const url = new URL(request.url);
  const requestToken = request.headers.get("x-oss-setup-token") ?? url.searchParams.get("setup_token");
  return requestToken === expectedToken;
}

export function webEditionFromEnv(value = process.env.NEXT_PUBLIC_PRISM_EDITION): WebEdition {
  if (value === undefined || value === "") {
    return "oss";
  }
  if (value === "oss" || value === "full") {
    return value;
  }
  throw new Error(`Unsupported NEXT_PUBLIC_PRISM_EDITION: ${value}`);
}

export function resolveSignupPolicy(options: SignupPolicyOptions = {}): SignupPolicy {
  const webEdition = options.webEdition ?? webEditionFromEnv();
  if (webEdition !== "oss") {
    return { registrationClosed: false };
  }
  if (!options.database && process.env.NEXT_PHASE === "phase-production-build") {
    return { registrationClosed: false };
  }

  const database = options.database ?? createSQLiteDatabase();
  const shouldClose = !options.database;
  try {
    const row = database.prepare("SELECT count(*) AS count FROM organizations WHERE deleted_at IS NULL").get() as { count: number | bigint };
    return { registrationClosed: Number(row.count) > 0 };
  } finally {
    if (shouldClose) {
      database.close();
    }
  }
}
