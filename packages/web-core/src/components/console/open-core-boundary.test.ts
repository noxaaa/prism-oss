import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { describe, expect, it } from "vitest";

function walkSourceFiles(root: string): string[] {
  const files: string[] = [];
  for (const entry of readdirSync(root)) {
    const path = join(root, entry);
    const stat = statSync(path);
    if (stat.isDirectory()) {
      files.push(...walkSourceFiles(path));
      continue;
    }
    if (/\.(ts|tsx)$/.test(path) && !path.includes(".test.")) {
      files.push(path);
    }
  }
  return files;
}

describe("OSS web core boundary", () => {
  it("does not include commercial-only console concepts", () => {
    for (const file of walkSourceFiles(join(process.cwd(), "src"))) {
      const relativePath = relative(process.cwd(), file).split(sep).join("/");
      const source = readFileSync(file, "utf8");
      expect(source, relativePath).not.toContain("/console/admin/rbac");
      expect(source, relativePath).not.toContain("nav.rbac");
      expect(source, relativePath).not.toContain("members.read");
      expect(source, relativePath).not.toContain("roles.read");
      expect(source, relativePath).not.toContain("multi_user");
    }
  });
});
