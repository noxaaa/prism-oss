import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { I18nProvider } from "./i18n";
import { ResourceMultiSelect, ResourceSelect } from "./resource-select";

describe("resource selectors", () => {
  it("render platform resource references as option components without text ID inputs", () => {
    const markup = renderToStaticMarkup(
      <I18nProvider>
        <form>
          <ResourceSelect
            label="Node group"
            onValueChange={() => undefined}
            options={[{ value: "ng_01", label: "Edge Group A" }]}
            value="ng_01"
          />
          <ResourceMultiSelect
            label="Roles"
            onValueChange={() => undefined}
            options={[{ value: "role_01", label: "Rule manager" }]}
            values={["role_01"]}
          />
        </form>
      </I18nProvider>,
    );

    expect(markup).toContain("data-resource-selector=\"single\"");
    expect(markup).toContain("data-resource-selector=\"multi\"");
    expect(markup).not.toMatch(/<input[^>]+name="(node_group_id|node_group_ids|target_id|target_group_id|role_ids|member_id|monitor_group_id|rule_id|listen_ip_id)"/);
    expect(markup).not.toMatch(/placeholder="[^"]*(resource|node|target|role|member|monitor|rule)[ _-]?id/i);
  });

  it("uses custom multi-select badges instead of the default selected marker", () => {
    const markup = renderToStaticMarkup(
      <I18nProvider>
        <ResourceMultiSelect
          badges={{
            role_01: { label: "Existing", variant: "secondary" },
            role_02: { label: "Removed", variant: "destructive" },
          }}
          label="Roles"
          onValueChange={() => undefined}
          options={[
            { value: "role_01", label: "Owner" },
            { value: "role_02", label: "Admin" },
          ]}
          values={["role_01"]}
        />
      </I18nProvider>,
    );

    expect(markup).toContain("Existing");
    expect(markup).toContain("Removed");
    expect(markup).not.toContain("Selected");
  });
});
