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

  it("exposes dataplane choice without raw HAProxy or nftables config inputs", () => {
    const text = [
      readFileSync(join(process.cwd(), "src/components/console/features/rules.tsx"), "utf8"),
      readFileSync(join(process.cwd(), "src/components/console/features/rule-mutation-form.tsx"), "utf8"),
    ].join("\n");

    expect(text).toContain("dataplane_preference");
    expect(text).toContain("ruleDataplanePreferenceOptions");
    expect(text).not.toContain("name=\"haproxy_config\"");
    expect(text).not.toContain("name=\"nftables_config\"");
  });

  it("renders port segment editor and send IP selector from node-group options", () => {
    const text = [
      readFileSync(join(process.cwd(), "src/components/console/features/rules.tsx"), "utf8"),
      readFileSync(join(process.cwd(), "src/components/console/features/rule-mutation-form.tsx"), "utf8"),
    ].join("\n");

    expect(text).toContain("port_segments");
    expect(text).toContain("send_ip");
    expect(text).toContain("node-group-send-ips");
    expect(text).toContain("defaultSendIPOptionValue");
    expect(text).toContain("buildListenIPOptionsURL");
    expect(text).toContain("PortSegmentsEditor");
    expect(text).toContain("formatPortSegments");
    expect(text).toContain("legacyPortForSegments(normalizedSegments)");
    expect(text).toContain("sendIPs.loading");
    expect(text).not.toContain("name=\"send_ip\"");
  });
});
