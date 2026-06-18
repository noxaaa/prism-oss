import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import { localeCandidatesFromAcceptLanguage, localeStorageKey, resolveLocale } from "@/components/console/i18n-core";
import "./globals.css";

export const metadata: Metadata = {
  title: process.env.APP_NAME ?? "APP_NAME",
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const cookieStore = await cookies();
  const headerStore = await headers();
  const locale = resolveLocale(cookieStore.get(localeStorageKey)?.value, localeCandidatesFromAcceptLanguage(headerStore.get("accept-language")));

  return (
    <html lang={locale} className="font-sans">
      <body>{children}</body>
    </html>
  );
}
