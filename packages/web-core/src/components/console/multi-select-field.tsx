"use client";

import { Field, FieldLabel } from "@noxaaa/prism-oss-web-core/ui/field";
import type { ResourceOption } from "@noxaaa/prism-oss-web-core/console/types";
import { FieldRequirementBadge } from "@noxaaa/prism-oss-web-core/console/shared";

export function MultiSelectField({ label, name, options, defaultValues = [], required = true }: { label: string; name: string; options: ResourceOption[]; defaultValues?: string[]; required?: boolean }) {
  return (
    <Field>
      <FieldLabel htmlFor={name}>{label}<FieldRequirementBadge required={required} /></FieldLabel>
      <select className="min-h-24 rounded-md border bg-background px-3 py-2 text-sm" defaultValue={defaultValues} id={name} multiple name={name} required={required} size={Math.min(Math.max(options.length, 3), 8)}>
        {options.map((option) => <option disabled={option.disabled} key={option.value} title={option.disabled_reason ?? option.disabledReason} value={option.value}>{option.label}</option>)}
      </select>
    </Field>
  );
}
