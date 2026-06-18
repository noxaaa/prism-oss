"use client";

import { CheckIcon } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldSet, FieldLegend } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ResourceOption } from "@/components/console/types";
import { normalizeOptions } from "@/components/console/control-api";
import { useI18n } from "@/components/console/i18n";

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
  value,
}: {
  description?: string;
  label: string;
  onValueChange: (value: string) => void;
  options: ResourceOption[];
  placeholder?: string;
  value: string;
}) {
  const { t } = useI18n();
  const normalized = normalizeOptions(options);
  const enabledOptions = normalized.filter((option) => !option.disabled);

  return (
    <Field data-resource-selector="single">
      <FieldLabel>{label}</FieldLabel>
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
  values,
}: {
  badges?: Record<string, ResourceMultiSelectBadge>;
  description?: string;
  label: string;
  onValueChange: (values: string[]) => void;
  options: ResourceOption[];
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
      <FieldLegend variant="label">{label}</FieldLegend>
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
