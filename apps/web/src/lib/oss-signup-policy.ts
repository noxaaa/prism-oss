import { getPostgresPool } from "./auth";

export type WebEdition = "oss" | "full";

export type SignupPolicy = {
  registrationClosed: boolean;
};

type SignupPolicyOptions = {
  database?: PolicyDatabase;
  webEdition?: WebEdition;
  closeDatabase?: boolean;
};

type PolicyDatabase = {
  query(sql: string): Promise<{ rows: Array<{ count?: number | string | bigint }> }>;
  end(): Promise<void>;
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

export async function resolveSignupPolicy(options: SignupPolicyOptions = {}): Promise<SignupPolicy> {
  const webEdition = options.webEdition ?? webEditionFromEnv();
  if (webEdition !== "oss") {
    return { registrationClosed: false };
  }
  if (!options.database && process.env.NEXT_PHASE === "phase-production-build") {
    return { registrationClosed: false };
  }

  const database = options.database ?? getPostgresPool();
  try {
    const result = await database.query("SELECT count(*) AS count FROM app.organizations WHERE deleted_at IS NULL");
    const row = result.rows[0] as { count: number | string | bigint } | undefined;
    return { registrationClosed: Number(row?.count ?? 0) > 0 };
  } finally {
    if (options.closeDatabase) {
      await database.end();
    }
  }
}
