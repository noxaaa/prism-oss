import { describe, expect, it } from "vitest";
import { buildResourceOptions } from "./resource-options";

describe("buildResourceOptions", () => {
  it("filters resources by organization and permission scope", () => {
    const options = buildResourceOptions({
      organizationId: "org_1",
      allowedResourceIds: new Set(["ng_01"]),
      resources: [
        { id: "ng_01", organizationId: "org_1", name: "Node Group A", enabled: true },
        { id: "ng_02", organizationId: "org_1", name: "Node Group B", enabled: true },
        { id: "ng_03", organizationId: "org_2", name: "Other Org", enabled: true },
      ],
    });

    expect(options).toEqual([
      {
        value: "ng_01",
        label: "Node Group A",
        disabled: false,
        disabledReason: undefined,
      },
    ]);
  });

  it("keeps disabled authorized resources visible with a reason", () => {
    const options = buildResourceOptions({
      organizationId: "org_1",
      allowedResourceIds: new Set(["ng_01"]),
      includeDisabledAuthorized: true,
      resources: [
        { id: "ng_01", organizationId: "org_1", name: "Node Group A", enabled: false },
      ],
    });

    expect(options).toEqual([
      {
        value: "ng_01",
        label: "Node Group A",
        disabled: true,
        disabledReason: "Resource is disabled",
      },
    ]);
  });

  it("honors wildcard scopes for high-privilege system options", () => {
    const options = buildResourceOptions({
      organizationId: "org_1",
      allowedResourceIds: new Set(["*"]),
      resources: [
        { id: "ng_01", organizationId: "org_1", name: "Node Group A", enabled: true },
        { id: "ng_02", organizationId: "org_1", name: "Node Group B", enabled: true },
        { id: "ng_03", organizationId: "org_2", name: "Other Org", enabled: true },
      ],
    });

    expect(options.map((option) => option.value)).toEqual(["ng_01", "ng_02"]);
  });

  it("never creates free-form options from user input", () => {
    const options = buildResourceOptions({
      organizationId: "org_1",
      allowedResourceIds: new Set(["ng_01"]),
      searchText: "manually typed id",
      resources: [],
    });

    expect(options).toEqual([]);
  });
});
