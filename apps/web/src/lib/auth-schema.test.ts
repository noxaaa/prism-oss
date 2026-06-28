import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { afterEach, describe, expect, it } from "vitest";
import { betterAuth } from "better-auth";
import { getMigrations } from "better-auth/db/migration";
import { Pool } from "pg";
import { buildAuthOptions, parseTrustedOrigins, resolvePostgresDatabaseURL } from "./auth";

const repoRoot = path.resolve(__dirname, "../../../..");
const migrationTestTimeoutMs = 90_000;
const originalEnv = { ...process.env };

describe("BetterAuth schema", () => {
  afterEach(() => {
    process.env = { ...originalEnv };
  });

  it("requires PostgreSQL URLs outside production build", () => {
    expect(resolvePostgresDatabaseURL("postgres://user:pass@localhost:5432/prism")).toBe("postgres://user:pass@localhost:5432/prism");
    process.env.NODE_ENV = "production";
    process.env.NEXT_PHASE = "";
    expect(() => resolvePostgresDatabaseURL("")).toThrow("DATABASE_URL is required");
  });

  it("normalizes configured trusted origins", () => {
    expect(parseTrustedOrigins(undefined)).toBeUndefined();
    expect(parseTrustedOrigins(" http://localhost:3001, http://127.0.0.1:3001 ,,")).toEqual([
      "http://localhost:3001",
      "http://127.0.0.1:3001",
    ]);
  });

  it("trusts the incoming request origin for self-hosted OSS access", async () => {
    process.env.BETTER_AUTH_URL = "http://127.0.0.1:3000";
    process.env.PUBLIC_WEB_URL = "http://127.0.0.1:3000";
    process.env.BETTER_AUTH_TRUSTED_ORIGINS = "";

    const pool = new Pool({ connectionString: "postgres://prism:prism@127.0.0.1:5432/prism" });
    try {
      const options = buildAuthOptions(pool);
      expect(typeof options.trustedOrigins).toBe("function");
      if (typeof options.trustedOrigins !== "function") {
        throw new Error("trustedOrigins must be dynamic");
      }

      expect(options.trustedOrigins(new Request("http://109.123.229.21:3000/api/auth/sign-up/email"))).toContain(
        "http://109.123.229.21:3000",
      );
    } finally {
      await pool.end();
    }
  });

	it("trusts the forwarded browser origin behind a reverse proxy", async () => {
		process.env.BETTER_AUTH_URL = "http://127.0.0.1:3000";
		process.env.PUBLIC_WEB_URL = "http://127.0.0.1:3000";
		process.env.BETTER_AUTH_TRUSTED_ORIGINS = "https://console.example.test,https://console.example.test:8443";

		const pool = new Pool({ connectionString: "postgres://prism:prism@127.0.0.1:5432/prism" });
    try {
      const options = buildAuthOptions(pool);
      expect(typeof options.trustedOrigins).toBe("function");
      if (typeof options.trustedOrigins !== "function") {
        throw new Error("trustedOrigins must be dynamic");
      }

      const request = new Request("http://127.0.0.1:3000/api/auth/sign-in/email", {
        headers: {
          "x-forwarded-host": "CONSOLE.EXAMPLE.TEST:443",
          "x-forwarded-proto": "https",
        },
      });
      expect(options.trustedOrigins(request)).toContain("https://console.example.test");

      const forwardedPortRequest = new Request("http://127.0.0.1:3000/api/auth/sign-in/email", {
        headers: {
          "x-forwarded-host": "console.example.test",
          "x-forwarded-port": "8443",
          "x-forwarded-proto": "https",
        },
      });
      expect(options.trustedOrigins(forwardedPortRequest)).toContain("https://console.example.test:8443");

      const canonicalRequest = new Request("http://127.0.0.1:3000/api/auth/sign-in/email", {
        headers: {
          "x-forwarded-host": "CONSOLE.EXAMPLE.TEST:443",
          "x-forwarded-proto": "https",
        },
      });
      expect(options.trustedOrigins(canonicalRequest)).toContain("https://console.example.test");

      const untrustedRequest = new Request("http://127.0.0.1:3000/api/auth/sign-in/email", {
        headers: {
          "x-forwarded-host": "attacker.example.test",
          "x-forwarded-proto": "https",
        },
      });
      expect(options.trustedOrigins(untrustedRequest)).not.toContain("https://attacker.example.test");
    } finally {
      await pool.end();
    }
  });

  it("does not trust forwarded browser origins unless trusted proxy headers are enabled", async () => {
    process.env.BETTER_AUTH_URL = "http://127.0.0.1:3000";
    process.env.PUBLIC_WEB_URL = "http://127.0.0.1:3000";
    process.env.BETTER_AUTH_TRUSTED_ORIGINS = "";
    process.env.BETTER_AUTH_TRUST_PROXY_HEADERS = "";

    const pool = new Pool({ connectionString: "postgres://prism:prism@127.0.0.1:5432/prism" });
    try {
      const options = buildAuthOptions(pool);
      expect(typeof options.trustedOrigins).toBe("function");
      if (typeof options.trustedOrigins !== "function") {
        throw new Error("trustedOrigins must be dynamic");
      }

      const request = new Request("http://127.0.0.1:3000/api/auth/sign-in/email", {
        headers: {
          "x-forwarded-host": "evil.example.test",
          "x-forwarded-proto": "https",
        },
      });
      expect(options.trustedOrigins(request)).not.toContain("https://evil.example.test");
    } finally {
      await pool.end();
    }
  });

  it("tracks the official BetterAuth PostgreSQL migration output", () => {
    const databaseURL = testDatabaseURL();
    if (!databaseURL) {
      return;
    }
    const generated = execFileSync("npm", ["--workspace", "apps/web", "run", "export:betterauth-schema", "--silent"], {
      cwd: repoRoot,
      env: {
        ...process.env,
        TEST_DATABASE_URL: databaseURL,
      },
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
      timeout: migrationTestTimeoutMs,
    });
    const tracked = readFileSync(path.join(repoRoot, "migrations/auth/00001_betterauth.sql"), "utf8");
    expect(normalizeSQL(tracked)).toBe(normalizeSQL(generated));
  }, migrationTestTimeoutMs);

  it("matches the schema generated by BetterAuth after goose migrations run", async () => {
    const migrated = await createMigratedTestDatabase();
    if (!migrated) {
      return;
    }
    const pool = new Pool({
      connectionString: migrated.url,
      options: "-c search_path=auth,public",
    });
    try {
      const migrations = await getMigrations(buildAuthOptions(pool));

      expect(migrations.toBeCreated).toEqual([]);
      expect(migrations.toBeAdded).toEqual([]);
    } finally {
      await pool.end();
      await migrated.drop();
    }
  }, migrationTestTimeoutMs);

  it("supports email and password sign-up and sign-in against migrated PostgreSQL", async () => {
    const migrated = await createMigratedTestDatabase();
    if (!migrated) {
      return;
    }
    const pool = new Pool({
      connectionString: migrated.url,
      options: "-c search_path=auth,public",
    });
    try {
      const testAuth = betterAuth(buildAuthOptions(pool));
      const email = "owner@example.com";
      const password = "correct-horse-password";

      const signUp = await postAuth(testAuth.handler, "/sign-up/email", {
        name: "Owner",
        email,
        password,
      });
      expect(signUp.status).toBe(200);

      const signIn = await postAuth(testAuth.handler, "/sign-in/email", {
        email,
        password,
      });
      expect(signIn.status).toBe(200);

      const user = await pool.query('SELECT email, "emailVerified" FROM "user" WHERE email = $1', [email]);
      expect(user.rows[0]).toMatchObject({ email, emailVerified: false });

      const account = await pool.query('SELECT "providerId", "userId" FROM "account" WHERE "providerId" = $1', ["credential"]);
      expect(account.rows[0].providerId).toBe("credential");
      expect(account.rows[0].userId).toBeTruthy();

      const session = await pool.query('SELECT count(*) AS count FROM "session"');
      expect(Number(session.rows[0].count)).toBeGreaterThanOrEqual(1);
    } finally {
      await pool.end();
      await migrated.drop();
    }
  }, migrationTestTimeoutMs);

  it("accepts self-hosted public origins even when the configured base URL is loopback", async () => {
    process.env.BETTER_AUTH_URL = "http://127.0.0.1:3000";
    process.env.PUBLIC_WEB_URL = "http://127.0.0.1:3000";
    process.env.BETTER_AUTH_TRUSTED_ORIGINS = "";

    const migrated = await createMigratedTestDatabase();
    if (!migrated) {
      return;
    }
    const pool = new Pool({
      connectionString: migrated.url,
      options: "-c search_path=auth,public",
    });
    try {
      const testAuth = betterAuth(buildAuthOptions(pool));

      const signUp = await postAuth(
        testAuth.handler,
        "/sign-up/email",
        {
          name: "Public Owner",
          email: "public-owner@example.com",
          password: "correct-horse-password",
        },
        "http://109.123.229.21:3000",
      );

      expect(signUp.status).toBe(200);
    } finally {
      await pool.end();
      await migrated.drop();
    }
  }, migrationTestTimeoutMs);
});

function testDatabaseURL(): string | undefined {
  return process.env.TEST_DATABASE_URL ?? process.env.DATABASE_URL;
}

async function createMigratedTestDatabase(): Promise<{ url: string; drop: () => Promise<void> } | undefined> {
  const baseURL = testDatabaseURL();
  if (!baseURL) {
    return undefined;
  }
  const databaseName = `prism_test_${randomUUID().replaceAll("-", "_")}`;
  const adminURL = new URL(baseURL);
  const testURL = new URL(baseURL);
  testURL.pathname = `/${databaseName}`;

  const admin = new Pool({ connectionString: adminURL.toString() });
  await admin.query(`CREATE DATABASE ${quoteIdentifier(databaseName)}`);

  try {
    execFileSync("go", ["run", "./cmd/migrate", "-database", testURL.toString(), "-dir", "migrations/auth,migrations/core", "up"], {
      cwd: repoRoot,
      env: {
        ...process.env,
        BETTER_AUTH_URL: "http://localhost:3000",
        GOCACHE: path.join(repoRoot, ".cache", "go-build"),
      },
      stdio: "pipe",
      timeout: 60_000,
    });
  } catch (error) {
    await admin.query(`DROP DATABASE IF EXISTS ${quoteIdentifier(databaseName)} WITH (FORCE)`);
    await admin.end();
    throw error;
  }
  await admin.end();

  return {
    url: testURL.toString(),
    async drop() {
      const dropAdmin = new Pool({ connectionString: adminURL.toString() });
      try {
        await dropAdmin.query(`DROP DATABASE IF EXISTS ${quoteIdentifier(databaseName)} WITH (FORCE)`);
      } finally {
        await dropAdmin.end();
      }
    },
  };
}

function quoteIdentifier(value: string): string {
  return `"${value.replaceAll('"', '""')}"`;
}

function normalizeSQL(value: string): string {
  return value
    .replace(/^-- Generated by.*$/gm, "-- Generated by apps/web/scripts/export-betterauth-schema.mjs")
    .replace(/\s+/g, " ")
    .trim()
    .toLowerCase();
}

async function postAuth(
  handler: (request: Request) => Promise<Response>,
  pathName: string,
  body: Record<string, unknown>,
  origin = "http://localhost:3000",
): Promise<Response> {
  return handler(
    new Request(`${origin}/api/auth${pathName}`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        origin,
      },
      body: JSON.stringify(body),
    }),
  );
}
