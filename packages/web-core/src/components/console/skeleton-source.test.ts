import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const consoleSource = (path: string) => readFileSync(join(process.cwd(), "src/components/console", path), "utf8");

describe("console skeleton source", () => {
  it("keeps shared skeleton primitives available for console pages", () => {
    const shared = consoleSource("shared/index.tsx");
    const shell = consoleSource("shell.tsx");

    expect(shared).toContain("export function TableSkeleton");
    expect(shared).toContain("export function CardTableSkeleton");
    expect(shared).toContain("loadingFallback");
    expect(shell).toContain("CardTableSkeleton");
    expect(shell).toContain("lg:grid-cols-[240px_1fr]");
    expect(shell).not.toContain("h-[560px] w-full");
  });

  it("uses explicit table skeleton fallbacks for feature table loading states", () => {
    const featureFiles = [
      "features/dns.tsx",
      "features/monitors.tsx",
      "features/nodes.tsx",
      "features/overview.tsx",
      "features/rules.tsx",
      "features/targets.tsx",
      "features/usage.tsx",
    ];
    const missingFallbacks = featureFiles.flatMap((file) => {
      const source = consoleSource(file);
      return [...source.matchAll(/<DataState\s+[^>]*loading=\{[^}]+\}[^>]*>/g)]
        .filter((match) => !match[0].includes("loadingFallback"))
        .map((match) => `${file}: ${match[0]}`);
    });

    expect(missingFallbacks).toEqual([]);
  });
});
