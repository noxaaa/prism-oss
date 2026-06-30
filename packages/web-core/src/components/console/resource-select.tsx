"use client";

import { CheckIcon } from "lucide-react";
import { Badge } from "@noxaaa/prism-oss-web-core/ui/badge";
import { Checkbox } from "@noxaaa/prism-oss-web-core/ui/checkbox";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldSet, FieldLegend } from "@noxaaa/prism-oss-web-core/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@noxaaa/prism-oss-web-core/ui/select";
import type { ResourceOption } from "@noxaaa/prism-oss-web-core/console/types";
import { normalizeOptions } from "@noxaaa/prism-oss-web-core/console/control-api";
import { useI18n } from "@noxaaa/prism-oss-web-core/console/i18n";
import { FieldRequirementBadge } from "@noxaaa/prism-oss-web-core/console/shared";

type ResourceMultiSelectBadge = {
  label: string;
  variant?: "default" | "secondary" | "destructive" | "outline";
};

export function ResourceSelect({
  description,
  label,
  onValueChange,
  options,
  placeholder,
  required = true,
  value,
}: {
  description?: string;
  label: string;
  onValueChange: (value: string) => void;
  options: ResourceOption[];
  placeholder?: string;
  required?: boolean;
  value: string;
}) {
  const { t } = useI18n();
  const normalized = normalizeOptions(options);
  const enabledOptions = normalized.filter((option) => !option.disabled);

  return (
    <Field data-resource-selector="single">
      <FieldLabel>{label}<FieldRequirementBadge required={required} /></FieldLabel>
      <Select disabled={enabledOptions.length === 0} onValueChange={onValueChange} value={value}>
        <SelectTrigger className="w-full">
          <SelectValue placeholder={enabledOptions.length === 0 ? t("resource.noAvailableOptions") : (placeholder ?? t("resource.selectResource"))} />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {normalized.map((option) => (
              <SelectItem disabled={option.disabled} key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <FieldDescription>{enabledOptions.length === 0 ? t("resource.noAuthorizedResources") : description}</FieldDescription>
    </Field>
  );
}

export function ResourceMultiSelect({
  badges = {},
  description,
  label,
  onValueChange,
  options,
  required = true,
  values,
}: {
  badges?: Record<string, ResourceMultiSelectBadge>;
  description?: string;
  label: string;
  onValueChange: (values: string[]) => void;
  options: ResourceOption[];
  required?: boolean;
  values: string[];
}) {
  const { t } = useI18n();
  const normalized = normalizeOptions(options);
  const selected = new Set(values);
  const enabledOptions = normalized.filter((option) => !option.disabled);

  function toggle(value: string, checked: boolean) {
    const next = new Set(selected);
    if (checked) {
      next.add(value);
    } else {
      next.delete(value);
    }
    onValueChange([...next]);
  }

  return (
    <FieldSet data-resource-selector="multi">
      <FieldLegend variant="label">{label}<FieldRequirementBadge required={required} /></FieldLegend>
      <FieldDescription>{enabledOptions.length === 0 ? t("resource.noAuthorizedResources") : description}</FieldDescription>
      <FieldGroup className="gap-2">
        {normalized.map((option) => {
          const badge = badges[option.value];
          return (
            <Field data-disabled={option.disabled ? true : undefined} key={option.value} orientation="horizontal">
              <Checkbox
                checked={selected.has(option.value)}
                disabled={option.disabled}
                id={`resource-${label}-${option.value}`}
                onCheckedChange={(checked) => toggle(option.value, checked === true)}
              />
              <FieldLabel className="font-normal" htmlFor={`resource-${label}-${option.value}`}>
                {option.label}
                {badge ? (
                  <Badge variant={badge.variant}>{badge.label}</Badge>
                ) : selected.has(option.value) ? (
                  <Badge variant="secondary">
                    <CheckIcon />
                    {t("common.selected")}
                  </Badge>
                ) : null}
              </FieldLabel>
            </Field>
          );
        })}
      </FieldGroup>
    </FieldSet>
  );
}
