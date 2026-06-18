import Database, { type Database as SQLiteDatabase } from "better-sqlite3";
import { betterAuth, type BetterAuthOptions } from "better-auth";

export function resolveSQLiteDatabasePath(databaseURL = process.env.DATABASE_URL): string {
  if (!databaseURL) {
    if (process.env.NODE_ENV === "test") {
      return ":memory:";
    }
    if (process.env.NEXT_PHASE === "phase-production-build") {
      return ":memory:";
    }
    throw new Error("DATABASE_URL is required for BetterAuth SQLite storage");
  }
  if (databaseURL.startsWith("sqlite://")) {
    return databaseURL.slice("sqlite://".length);
  }
  return databaseURL;
}

export function createSQLiteDatabase(databasePath = resolveSQLiteDatabasePath()): SQLiteDatabase {
  const database = new Database(databasePath, { timeout: 5000 });
  database.pragma("foreign_keys = ON");
  if (databasePath !== ":memory:") {
    database.pragma("journal_mode = WAL");
  }
  return database;
}

export function parseTrustedOrigins(value = process.env.BETTER_AUTH_TRUSTED_ORIGINS): string[] | undefined {
  const origins =
    value
      ?.split(",")
      .map((origin) => origin.trim())
      .filter(Boolean) ?? [];
  return origins.length > 0 ? origins : undefined;
}

function originFromURL(value: string | null | undefined): string | undefined {
  if (!value) {
    return undefined;
  }
  try {
    return new URL(value).origin;
  } catch {
    return undefined;
  }
}

function uniqueOrigins(origins: Array<string | undefined>): string[] | undefined {
  const unique = [...new Set(origins.filter((origin): origin is string => Boolean(origin)))];
  return unique.length > 0 ? unique : undefined;
}

export function resolveAuthBaseURL(): string | undefined {
  const isNextProductionBuild = process.env.NEXT_PHASE === "phase-production-build";
  return (
    process.env.BETTER_AUTH_URL ??
    process.env.PUBLIC_WEB_URL ??
    (process.env.NODE_ENV === "test" || isNextProductionBuild ? "http://localhost:3000" : undefined)
  );
}

export function resolveTrustedOrigins(request?: Request): string[] | undefined {
  return uniqueOrigins([
    ...(parseTrustedOrigins() ?? []),
    originFromURL(process.env.BETTER_AUTH_URL),
    originFromURL(process.env.PUBLIC_WEB_URL),
    originFromURL(request?.url),
  ]);
}

export function buildAuthOptions(database: SQLiteDatabase = createSQLiteDatabase()): BetterAuthOptions {
  return {
    database,
    secret:
      process.env.BETTER_AUTH_SECRET ??
      (process.env.NODE_ENV === "test"
        ? "test-better-auth-secret-32-bytes"
        : process.env.NEXT_PHASE === "phase-production-build"
          ? "build-better-auth-secret-32-bytes"
          : undefined),
    baseURL: resolveAuthBaseURL(),
    trustedOrigins: (request?: Request) => resolveTrustedOrigins(request) ?? [],
    emailAndPassword: {
      enabled: true,
    },
  };
}

export const auth = betterAuth(buildAuthOptions());
