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

export function buildAuthOptions(database: SQLiteDatabase = createSQLiteDatabase()): BetterAuthOptions {
  const isNextProductionBuild = process.env.NEXT_PHASE === "phase-production-build";
  const trustedOrigins = parseTrustedOrigins();
  return {
    database,
    secret:
      process.env.BETTER_AUTH_SECRET ??
      (process.env.NODE_ENV === "test"
        ? "test-better-auth-secret-32-bytes"
        : isNextProductionBuild
          ? "build-better-auth-secret-32-bytes"
          : undefined),
    baseURL: process.env.BETTER_AUTH_URL ?? (process.env.NODE_ENV === "test" || isNextProductionBuild ? "http://localhost:3000" : undefined),
    trustedOrigins,
    emailAndPassword: {
      enabled: true,
    },
  };
}

export const auth = betterAuth(buildAuthOptions());
