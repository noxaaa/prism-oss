import { describe, expect, it } from "vitest";
import { healthProbeConfigFromForm, healthProbeSchemeDefault } from "./features/monitors";

function form(entries: Array<[string, string]>): FormData {
  const data = new FormData();
  for (const [key, value] of entries) data.set(key, value);
  return data;
}

describe("health probe config form helpers", () => {
  it("serializes TCP port override config", () => {
    expect(healthProbeConfigFromForm(form([["config_port_override", "8443"]]), "TCP_PORT")).toEqual({ port_override: 8443 });
  });

  it("serializes HTTP visual config fields", () => {
    const config = healthProbeConfigFromForm(form([
      ["config_port_override", "8443"],
      ["config_http_scheme", "https"],
      ["config_http_method", "POST"],
      ["config_http_path", "healthz"],
      ["config_http_expected_statuses", "200, 204 301,200"],
    ]), "HTTP");

    expect(config).toEqual({
      port_override: 8443,
      scheme: "https",
      method: "POST",
      path: "/healthz",
      expected_statuses: [200, 204, 301],
    });
  });

  it("preserves custom HTTP methods when editing existing checks", () => {
    const config = healthProbeConfigFromForm(form([
      ["config_http_method", "PROPFIND"],
    ]), "HTTP");

    expect(config).toEqual({ scheme: "http", method: "PROPFIND", path: "/" });
  });

  it("normalizes existing HTTP probe schemes for visual editing", () => {
    expect(healthProbeSchemeDefault({ scheme: "HTTPS" })).toBe("https");
    expect(healthProbeSchemeDefault({ scheme: "HtTpS" })).toBe("https");
    expect(healthProbeSchemeDefault({ scheme: "ftp" })).toBe("http");
    expect(healthProbeSchemeDefault({})).toBe("http");
  });

  it("omits unsupported and empty probe config values", () => {
    expect(healthProbeConfigFromForm(form([
      ["config_port_override", "70000"],
      ["config_http_expected_statuses", "abc, 99, 600"],
    ]), "HTTP")).toEqual({ scheme: "http", method: "GET", path: "/" });
    expect(healthProbeConfigFromForm(form([["config_port_override", "443"]]), "ICMP")).toEqual({});
  });
});
