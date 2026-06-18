import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import Database from "better-sqlite3";
import { describe, expect, it } from "vitest";

import { isAuthSignUpPath, resolveSignupPolicy } from "./oss-signup-policy";

describe("OSS signup policy", () => {
  it("allows OSS sign-up before the first organization exists", () => {
    const database = createPolicyDatabase();
    try {
      expect(resolveSignupPolicy({ database, webEdition: "oss" })).toEqual({ registrationClosed: false });
    } finally {
      database.close();
    }
  });

  it("disables OSS sign-up after the first organization exists", () => {
    const database = createPolicyDatabase();
    try {
      database.prepare("INSERT INTO organizations (id, name, slug) VALUES (?, ?, ?)").run("org_1", "Local", "local");

      expect(resolveSignupPolicy({ database, webEdition: "oss" })).toEqual({ registrationClosed: true });
    } finally {
      database.close();
    }
  });

  it("keeps full edition sign-up open after organizations exist", () => {
    const database = createPolicyDatabase();
    try {
      database.prepare("INSERT INTO organizations (id, name, slug) VALUES (?, ?, ?)").run("org_1", "Local", "local");

      expect(resolveSignupPolicy({ database, webEdition: "full" })).toEqual({ registrationClosed: false });
    } finally {
      database.close();
    }
  });

  it("matches only email sign-up auth paths", () => {
    expect(isAuthSignUpPath("http://localhost:3000/api/auth/sign-up/email")).toBe(true);
    expect(isAuthSignUpPath("http://localhost:3000/api/auth/sign-in/email")).toBe(false);
    expect(isAuthSignUpPath("http://localhost:3000/api/control/bootstrap")).toBe(false);
  });
});

function createPolicyDatabase(): Database.Database {
  const database = new Database(path.join(mkdtempSync(path.join(tmpdir(), "platform-oss-signup-")), "app.db"));
  database.exec(`
    CREATE TABLE organizations (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      slug TEXT NOT NULL UNIQUE,
      deleted_at TEXT
    );
  `);
  return database;
}
