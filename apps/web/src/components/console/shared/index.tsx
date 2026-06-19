"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { controlGet } from "@/components/console/control-api";
import { localizeControlError, localizeStatus, useI18n } from "@/components/console/i18n";
import type { MonitorGroup, NodeGroup, ResourceOption } from "@/components/console/types";

export function ResourceTable({
  description,
  error,
  headers,
  icon,
  loading,
  rows,
  title,
}: {
  description: string;
  error: string;
  headers: string[];
  icon: ReactNode;
  loading: boolean;
  rows: ReactNode[][];
  title: string;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <DataState emptyIcon={icon} loading={loading} error={error}>
          <Table>
            <TableHeader>
              <TableRow>{headers.map((header) => <TableHead key={header}>{header}</TableHead>)}</TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row, index) => (
                <TableRow key={index}>
                  {row.map((cell, cellIndex) => <TableCell key={cellIndex}>{cell}</TableCell>)}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DataState>
      </CardContent>
    </Card>
  );
}

export function DataState({
  children,
  emptyIcon,
  error,
  loading,
}: {
  children: ReactNode;
  emptyIcon?: ReactNode;
  error: string;
  loading: boolean;
}) {
  if (loading) {
    return (
      <div className="flex flex-col gap-3">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-11/12" />
        <Skeleton className="h-8 w-10/12" />
      </div>
    );
  }
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{error}</AlertTitle>
      </Alert>
    );
  }
  return (
    <div className="min-w-0 overflow-x-auto">
      {children}
    </div>
  );
}

export function PageStack({ children }: { children: ReactNode }) {
  return <div className="flex flex-col gap-6">{children}</div>;
}

export function SummaryGrid({ children }: { children: ReactNode }) {
  return <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">{children}</div>;
}

export function SummaryCard({ icon, label, value }: { icon: ReactNode; label: string; value: ReactNode }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          {icon}
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold">{value}</div>
      </CardContent>
    </Card>
  );
}

export function TextField({
  defaultValue,
  label,
  name,
  placeholder,
  required = true,
  type = "text",
}: {
  defaultValue?: string;
  label: string;
  name: string;
  placeholder: string;
  required?: boolean;
  type?: string;
}) {
  return (
    <Field>
      <FieldLabel htmlFor={name}>{label}</FieldLabel>
      <Input defaultValue={defaultValue} id={name} name={name} placeholder={placeholder} required={required} type={type} />
    </Field>
  );
}

export function ControlledTextField({
  label,
  onValueChange,
  placeholder,
  type = "text",
  value,
}: {
  label: string;
  onValueChange: (value: string) => void;
  placeholder: string;
  type?: string;
  value: string;
}) {
  const id = label.toLowerCase().replace(/\s+/g, "-");
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input id={id} onChange={(event) => onValueChange(event.currentTarget.value)} placeholder={placeholder} required type={type} value={value} />
    </Field>
  );
}

export function TextAreaField({ defaultValue, label, name, placeholder }: { defaultValue?: string; label: string; name: string; placeholder: string }) {
  return (
    <Field>
      <FieldLabel htmlFor={name}>{label}</FieldLabel>
      <Textarea defaultValue={defaultValue} id={name} name={name} placeholder={placeholder} />
    </Field>
  );
}

export function EnumSelect({
  label,
  onValueChange,
  options,
  value,
}: {
  label: string;
  onValueChange: (value: string) => void;
  options: Array<{ value: string; label: string }>;
  value: string;
}) {
  return (
    <Field>
      <FieldLabel>{label}</FieldLabel>
      <Select onValueChange={onValueChange} value={value}>
        <SelectTrigger className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {options.map((option) => (
              <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </Field>
  );
}

export function StatusBadge({ value }: { value: string }) {
  const { locale } = useI18n();
  const normalized = value.toUpperCase();
  const variant = normalized === "ONLINE" || normalized === "ENABLED" || normalized === "ACTIVE" ? "default" : normalized === "DISABLED" || normalized === "FAILED" ? "destructive" : "secondary";
  return <Badge variant={variant}>{localizeStatus(value, locale)}</Badge>;
}

export function percent(value: number | undefined): string {
  if (value === undefined) {
    return "0%";
  }
  return `${value.toFixed(1)}%`;
}

export function duration(value: number | undefined): string {
  const totalSeconds = Math.max(0, Math.floor(value ?? 0));
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (days > 0) {
    return `${days}d ${hours}h`;
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}

export function useControlList<T>(path: string) {
  const { locale } = useI18n();
  const [data, setData] = useState<T[]>([]);
  const [loading, setLoading] = useState(Boolean(path));
  const [error, setError] = useState("");

  const refresh = useMemo(
    () => async () => {
      if (!path) {
        setData([]);
        setLoading(false);
        return;
      }
      setLoading(true);
      setError("");
      try {
        const result = await controlGet<T[] | null>(path);
        setData(Array.isArray(result) ? result : []);
      } catch (requestError) {
        setData([]);
        setError(localizeControlError(requestError, locale));
      } finally {
        setLoading(false);
      }
    },
    [locale, path],
  );

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { data, error, loading, refresh };
}

export function ensureFirstValue(options: ResourceOption[], current: string, setValue: (value: string) => void) {
  const enabled = options.filter((option) => !option.disabled);
  if (enabled.length === 0) {
    if (current) {
      setValue("");
    }
    return;
  }
  if (!enabled.some((option) => option.value === current)) {
    setValue(enabled[0].value);
  }
}

export function groupName(groups: NodeGroup[], id: string): string {
  return groups.find((group) => group.id === id)?.name ?? id;
}

export function monitorGroupName(groups: MonitorGroup[], id: string): string {
  return groups.find((group) => group.id === id)?.name ?? id;
}

export async function copyText(value: string, successMessage = "Copied.") {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(successMessage);
    return;
  } catch {
    // Some browsers block Clipboard API outside secure contexts. Fall back to the legacy selection path.
  }

  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.setAttribute("readonly", "true");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.select();
  try {
    if (!document.execCommand("copy")) {
      throw new Error("copy failed");
    }
    toast.success(successMessage);
  } finally {
    document.body.removeChild(textarea);
  }
}
