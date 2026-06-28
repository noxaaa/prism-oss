import { betterAuth, type BetterAuthOptions } from "better-auth";
import { Pool, type PoolConfig } from "pg";

export type PostgresDatabase = Pool;

let sharedPool: Pool | undefined;

export function resolvePostgresDatabaseURL(databaseURL = process.env.DATABASE_URL): string {
  if (!databaseURL) {
    if (process.env.NODE_ENV === "test" || process.env.NEXT_PHASE === "phase-production-build") {
      return "postgres://prism:prism@127.0.0.1:5432/prism_build";
    }
    throw new Error("DATABASE_URL is required for BetterAuth PostgreSQL storage");
  }
  return databaseURL;
}

export function postgresPoolConfig(databaseURL = resolvePostgresDatabaseURL()): PoolConfig {
  return {
    connectionString: databaseURL,
    max: 10,
    idleTimeoutMillis: 30_000,
    connectionTimeoutMillis: 5_000,
    options: "-c search_path=auth,public",
  };
}

export function createPostgresPool(databaseURL = resolvePostgresDatabaseURL()): Pool {
  return new Pool(postgresPoolConfig(databaseURL));
}

export function getPostgresPool(): Pool {
  sharedPool ??= createPostgresPool();
  return sharedPool;
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

function firstForwardedValue(value: string | null | undefined): string | undefined {
  return value?.split(",")[0]?.trim() || undefined;
}

function isValidForwardedPort(port: string | undefined): port is string {
  if (!port || !/^\d{1,5}$/.test(port)) {
    return false;
  }
  const parsed = Number(port);
  return Number.isInteger(parsed) && parsed >= 1 && parsed <= 65_535;
}

function isValidForwardedHost(host: string): boolean {
  if (!host || /[\s<>'"\\/]|\.\.|^\./.test(host)) {
    return false;
  }
  try {
    const parsed = new URL(`http://${host}`);
    return parsed.host.toLowerCase() === host.toLowerCase() && parsed.pathname === "/" && !parsed.username && !parsed.password;
  } catch {
    return false;
  }
}

function forwardedHostWithPort(host: string, port: string | undefined): string {
  if (!isValidForwardedPort(port)) {
    return host;
  }
  const parsed = new URL(`http://${host}`);
  if (parsed.port) {
    return host;
  }
  return `${host}:${port}`;
}

function configuredTrustedOrigins(): string[] {
  return uniqueOrigins([
    ...(parseTrustedOrigins() ?? []),
    originFromURL(process.env.BETTER_AUTH_URL),
    originFromURL(process.env.PUBLIC_WEB_URL),
  ]) ?? [];
}

function forwardedOriginFromRequest(request: Request | undefined, allowedOrigins: string[]): string | undefined {
  const proto = firstForwardedValue(request?.headers.get("x-forwarded-proto"))?.toLowerCase();
  const host = firstForwardedValue(request?.headers.get("x-forwarded-host"));
  if ((proto !== "http" && proto !== "https") || !host || !isValidForwardedHost(host)) {
    return undefined;
  }
  const forwardedPort = firstForwardedValue(request?.headers.get("x-forwarded-port"));
  const forwardedOrigin = originFromURL(`${proto}://${forwardedHostWithPort(host, forwardedPort)}`);
  return forwardedOrigin && allowedOrigins.includes(forwardedOrigin) ? forwardedOrigin : undefined;
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
  const configuredOrigins = configuredTrustedOrigins();
  return uniqueOrigins([
    ...configuredOrigins,
    forwardedOriginFromRequest(request, configuredOrigins),
    originFromURL(request?.url),
  ]);
}

export function buildAuthOptions(database: PostgresDatabase = getPostgresPool()): BetterAuthOptions {
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
    advanced: {
      trustedProxyHeaders: process.env.BETTER_AUTH_TRUST_PROXY_HEADERS === "true",
    },
    trustedOrigins: (request?: Request) => resolveTrustedOrigins(request) ?? [],
    emailAndPassword: {
      enabled: true,
    },
  };
}

export const auth = betterAuth(buildAuthOptions());
