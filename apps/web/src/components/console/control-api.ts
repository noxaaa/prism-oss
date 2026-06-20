"use client";

import type { APIEnvelope, ResourceOption } from "@/components/console/types";
import { formatMessage, type Locale } from "@/components/console/i18n-core";

export class ControlAPIError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Record<string, unknown>;

  constructor(status: number, code: string, message?: string, details?: Record<string, unknown>) {
    super(message || code);
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

export async function controlGet<T>(path: string): Promise<T> {
  return controlRequest<T>(path, { method: "GET" });
}

export async function controlPost<T>(path: string, body: unknown): Promise<T> {
  return controlRequest<T>(path, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });
}

export async function controlPatch<T>(path: string, body: unknown): Promise<T> {
  return controlRequest<T>(path, {
    method: "PATCH",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });
}

export async function controlDelete<T>(path: string): Promise<T> {
  return controlRequest<T>(path, { method: "DELETE" });
}

export async function controlRequest<T>(path: string, init: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  const body = (await response.json().catch(() => ({}))) as APIEnvelope<T>;
  if (!response.ok) {
    throw new ControlAPIError(response.status, body.error?.code ?? "CONTROL_API_ERROR", body.error?.message, body.error?.details);
  }
  return body.data as T;
}

export function optionLabel(options: ResourceOption[], value: string): string {
  return options.find((option) => option.value === value)?.label ?? value;
}

export function normalizeOptions(options: ResourceOption[]): ResourceOption[] {
  return options.map((option) => ({
    ...option,
    disabled: option.disabled ?? false,
    disabled_reason: option.disabled_reason ?? option.disabledReason,
  }));
}

export function bytes(value: number | undefined): string {
	const amount = value ?? 0;
	if (amount < 1024) {
    return `${amount} B`;
  }
  if (amount < 1024 * 1024) {
    return `${(amount / 1024).toFixed(1)} KiB`;
  }
  if (amount < 1024 * 1024 * 1024) {
    return `${(amount / 1024 / 1024).toFixed(1)} MiB`;
  }
	return `${(amount / 1024 / 1024 / 1024).toFixed(1)} GiB`;
}

export function formatBitrateBps(value: number | null | undefined): string {
  const amount = Math.max(0, value ?? 0);
  if (amount === 0) {
    return "0 bps";
  }
  const units = ["bps", "Kbps", "Mbps", "Gbps", "Tbps"];
  let unitIndex = 0;
  let scaled = amount;
  while (scaled >= 1000 && unitIndex < units.length - 1) {
    scaled /= 1000;
    unitIndex += 1;
  }
  if (unitIndex === 0) {
    return `${Math.round(scaled)} ${units[unitIndex]}`;
  }
  return `${scaled.toFixed(1)} ${units[unitIndex]}`;
}

export function shortDate(value?: string, locale: Locale = "en"): string {
  if (!value) {
    return formatMessage(locale, "common.never");
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString(locale);
}
