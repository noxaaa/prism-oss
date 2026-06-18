export type PlatformResource = {
  id: string;
  organizationId: string;
  name: string;
  enabled: boolean;
  description?: string;
  status?: string;
};

export type ResourceOption = {
  value: string;
  label: string;
  disabled: boolean;
  disabledReason?: string;
};

export type BuildResourceOptionsInput = {
  organizationId: string;
  resources: PlatformResource[];
  allowedResourceIds: Set<string>;
  includeDisabledAuthorized?: boolean;
  searchText?: string;
};

export function buildResourceOptions(input: BuildResourceOptionsInput): ResourceOption[] {
  const canUseAllResources = input.allowedResourceIds.has("*");

  return input.resources
    .filter((resource) => resource.organizationId === input.organizationId)
    .filter((resource) => canUseAllResources || input.allowedResourceIds.has(resource.id))
    .filter((resource) => resource.enabled || input.includeDisabledAuthorized)
    .map((resource) => ({
      value: resource.id,
      label: resource.name,
      disabled: !resource.enabled,
      disabledReason: resource.enabled ? undefined : "Resource is disabled",
    }));
}
