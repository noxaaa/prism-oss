import { cookies, headers } from "next/headers";
import { localeCandidatesFromAcceptLanguage, localeStorageKey, resolveLocale, type Locale } from "@/components/console/i18n-core";
import { auth } from "@/lib/auth";
import { resolveSignupPolicy } from "@/lib/oss-signup-policy";

export type InitialControlUser = {
  id: string;
  email: string;
  name: string;
} | null;

export type ConsoleServerContext = {
  appName: string;
  initialUser: InitialControlUser;
  locale: Locale;
  registrationClosed: boolean;
};

export async function getConsoleServerContext(): Promise<ConsoleServerContext> {
  const headerStore = await headers();
  const cookieStore = await cookies();
  const session = await auth.api.getSession({ headers: headerStore });
  const locale = resolveLocale(cookieStore.get(localeStorageKey)?.value, localeCandidatesFromAcceptLanguage(headerStore.get("accept-language")));
  const initialUser =
    session?.user.id && session.user.email
      ? {
          id: session.user.id,
          email: session.user.email,
          name: session.user.name ?? "",
        }
      : null;

  return {
    appName: process.env.APP_NAME ?? "APP_NAME",
    initialUser,
    locale,
    registrationClosed: (await resolveSignupPolicy()).registrationClosed,
  };
}
