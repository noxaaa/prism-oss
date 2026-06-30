"use client";

import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@noxaaa/prism-oss-web-core/ui/select";
import {
  formatMessage,
  localeStorageKey,
  resolveLocale,
  type Locale,
  type MessageKey,
  type MessageParams,
  type MessagesByLocale,
} from "@noxaaa/prism-oss-web-core/console/i18n-core";

export {
  formatMessage,
  localeCandidatesFromAcceptLanguage,
  localizeControlError,
  localizeEnum,
  localizeImportIssue,
  localizeStatus,
  messages,
  resolveLocale,
} from "@noxaaa/prism-oss-web-core/console/i18n-core";
export type { Locale, MessageKey, MessageParams, MessagesByLocale } from "@noxaaa/prism-oss-web-core/console/i18n-core";

type I18nContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: MessageKey, params?: MessageParams) => string;
};

const I18nContext = createContext<I18nContextValue | null>(null);

export function I18nProvider({ children, initialLocale = "zh-CN", messages: messageOverrides }: { children?: ReactNode; initialLocale?: Locale; messages?: MessagesByLocale }) {
  const [locale, setLocaleState] = useState<Locale>(initialLocale);

  useEffect(() => {
    const resolved = preferredClientLocale(initialLocale);
    setLocaleState((current) => (current === resolved ? current : resolved));
    persistLocale(resolved);
  }, [initialLocale]);

  function setLocale(nextLocale: Locale) {
    setLocaleState(nextLocale);
    persistLocale(nextLocale);
  }

  const value = useMemo<I18nContextValue>(() => ({
    locale,
    setLocale,
    t: (key, params) => formatMessage(locale, key, params, messageOverrides),
  }), [locale, messageOverrides]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nContextValue {
  const value = useContext(I18nContext);
  if (!value) {
    throw new Error("useI18n must be used inside I18nProvider");
  }
  return value;
}

export function LanguageSwitch({ className }: { className?: string }) {
  const { locale, setLocale, t } = useI18n();
  return (
    <Select onValueChange={(value) => setLocale(value as Locale)} value={locale}>
      <SelectTrigger aria-label={t("common.language")} className={className ?? "w-32"}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectItem value="zh-CN">{t("common.chinese")}</SelectItem>
          <SelectItem value="en">{t("common.english")}</SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}

function preferredClientLocale(fallback: Locale): Locale {
  if (typeof window === "undefined") {
    return fallback;
  }
  const stored = window.localStorage.getItem(localeStorageKey) ?? readCookie(localeStorageKey);
  return resolveLocale(stored, window.navigator.languages.length > 0 ? window.navigator.languages : [fallback]);
}

function persistLocale(locale: Locale) {
  if (typeof document !== "undefined") {
    document.documentElement.lang = locale;
    document.cookie = `${localeStorageKey}=${encodeURIComponent(locale)}; path=/; max-age=31536000; SameSite=Lax`;
  }
  if (typeof window !== "undefined") {
    window.localStorage.setItem(localeStorageKey, locale);
  }
}

function readCookie(name: string): string | null {
  if (typeof document === "undefined") {
    return null;
  }
  const prefix = `${name}=`;
  const cookie = document.cookie.split("; ").find((part) => part.startsWith(prefix));
  return cookie ? decodeURIComponent(cookie.slice(prefix.length)) : null;
}
