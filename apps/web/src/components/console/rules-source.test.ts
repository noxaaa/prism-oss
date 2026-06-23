import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

describe("rules console source", () => {
  it("keeps long SNI hostnames out of the table cell and exposes them through a hover card", () => {
    const text = [
      readFileSync(join(process.cwd(), "src/components/console/features/rules.tsx"), "utf8"),
      readFileSync(join(process.cwd(), "src/components/console/rule-match-cell.tsx"), "utf8"),
    ].join("\n");

    expect(text).toContain("RuleMatchCell");
    expect(text).toContain("HoverCard");
    expect(text).not.toContain("`${rule.match.type, locale)} ${rule.match.sni_hostname}`");
    expect(text).not.toContain("{localizeEnum(rule.match.type, locale)}{rule.match.sni_hostname ? ` ${rule.match.sni_hostname}` : \"\"}");
  });
});
