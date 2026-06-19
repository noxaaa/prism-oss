"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  DatabaseIcon,
  KeyRoundIcon,
  LogOutIcon,
  UsersIcon,
} from "lucide-react";
import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Toaster } from "@/components/ui/sonner";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { ControlAPIError, controlGet, controlPost } from "@/components/console/control-api";
import { defaultConsoleRegistry, type ConsoleNavItem, type Workspace } from "@/components/console/edition-registry";
import { I18nProvider, LanguageSwitch, localizeControlError, useI18n, type Locale, type MessageKey } from "@/components/console/i18n";
import { canUseAdminWorkspace, hasAnyPermission, roleSummary } from "@/components/console/permissions";
import type { ControlSession, InitialUser } from "@/components/console/types";

type ConsoleSessionState = {
  error: string;
  errorCode: string;
  loading: boolean;
  refresh: () => Promise<void>;
  session: ControlSession | null;
};

const ConsoleSessionContext = createContext<ConsoleSessionState | null>(null);

export function useConsoleSession(): ConsoleSessionState {
  const value = useContext(ConsoleSessionContext);
  if (!value) {
    throw new Error("useConsoleSession must be used inside ConsoleShell");
  }
  return value;
}

export function ConsoleGateway({
  appName,
  initialLocale,
  initialUser,
  registrationClosed,
}: {
  appName: string;
  initialLocale: Locale;
  initialUser: InitialUser;
  registrationClosed: boolean;
}) {
  return (
    <I18nProvider initialLocale={initialLocale}>
      <ConsoleGatewayContent appName={appName} initialUser={initialUser} registrationClosed={registrationClosed} />
    </I18nProvider>
  );
}

function ConsoleGatewayContent({ appName, initialUser, registrationClosed }: { appName: string; initialUser: InitialUser; registrationClosed: boolean }) {
  return (
    <ConsoleSessionProvider initialUser={initialUser}>
      {(state) => {
        if (!initialUser) {
          return <AuthScreen appName={appName} registrationClosed={registrationClosed} />;
        }
        if (state.loading) {
          return <ConsoleLoading appName={appName} />;
        }
        if (!state.session?.organization?.id) {
          return <SetupScreen appName={appName} error={state.error} errorCode={state.errorCode} initialUser={initialUser} onReady={state.refresh} />;
        }
        return <DefaultWorkspaceRedirect appName={appName} session={state.session} />;
      }}
    </ConsoleSessionProvider>
  );
}

export function ConsoleShell({
  active,
  appName,
  children,
  initialLocale,
  initialUser,
  registrationClosed,
  title,
  titleKey,
  workspace,
}: {
  active: string;
  appName: string;
  children: ReactNode;
  initialUser: InitialUser;
  title: string;
  titleKey?: MessageKey;
  workspace: Workspace;
  initialLocale: Locale;
  registrationClosed: boolean;
}) {
  return (
    <I18nProvider initialLocale={initialLocale}>
      <ConsoleShellContent active={active} appName={appName} initialUser={initialUser} registrationClosed={registrationClosed} title={title} titleKey={titleKey} workspace={workspace}>
        {children}
      </ConsoleShellContent>
    </I18nProvider>
  );
}

function ConsoleShellContent({
  active,
  appName,
  children,
  initialUser,
  registrationClosed,
  title,
  titleKey,
  workspace,
}: {
  active: string;
  appName: string;
  children: ReactNode;
  initialUser: InitialUser;
  registrationClosed: boolean;
  title: string;
  titleKey?: MessageKey;
  workspace: Workspace;
}) {
  const { t } = useI18n();
  const showRBACChrome = hasConsoleCapability(defaultConsoleRegistry, "rbac");
  return (
    <ConsoleSessionProvider initialUser={initialUser}>
      {(state) => {
        if (!initialUser) {
          return <AuthScreen appName={appName} registrationClosed={registrationClosed} />;
        }
        if (state.loading) {
          return <ConsoleLoading appName={appName} />;
        }
        if (!state.session?.organization?.id) {
          return <SetupScreen appName={appName} error={state.error} errorCode={state.errorCode} initialUser={initialUser} onReady={state.refresh} />;
        }
        return (
          <ConsoleSessionContext.Provider value={state}>
            <TooltipProvider>
              <main className="min-h-screen bg-background text-foreground">
                <div className="grid min-h-screen lg:grid-cols-[240px_1fr]">
                  <aside className="border-r bg-card">
                    <div className="flex h-full flex-col gap-4 p-4">
                      <div className="flex items-center gap-3">
                        <div className="flex size-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
                          <DatabaseIcon />
                        </div>
                        <div className="min-w-0">
                          <div className="truncate text-sm font-medium">{appName}</div>
                          <div className="truncate text-xs text-muted-foreground">{state.session.organization.name}</div>
                        </div>
                      </div>
                      <WorkspaceSwitch session={state.session} workspace={workspace} />
                      <nav className="flex flex-col gap-1">
                        {defaultConsoleRegistry.itemsByWorkspace[workspace]
                          .filter((item) => canAccessNavItem(state.session, item))
                          .map((item) => (
                            <NavItem active={active === item.key} href={item.href} icon={item.icon} key={item.href} label={t(item.labelKey)} />
                          ))}
                      </nav>
                      <div className="mt-auto flex flex-col gap-3">
                        <Separator />
                        <div className="flex items-center gap-3">
                          <Avatar className="size-8">
                            <AvatarFallback>{initials(state.session.user.email)}</AvatarFallback>
                          </Avatar>
                          <div className="min-w-0 flex-1">
                            <div className="truncate text-sm font-medium">{state.session.user.email}</div>
                            {showRBACChrome && state.session.roles?.length ? (
                              <div className="truncate text-xs text-muted-foreground">{roleSummary(state.session)}</div>
                            ) : null}
                          </div>
                          <SignOutButton />
                        </div>
                      </div>
                    </div>
                  </aside>
                  <section className="min-w-0">
                    <header className="border-b bg-background/95 px-4 py-4 lg:px-8">
                      <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                        <div>
                          <p className="text-sm text-muted-foreground">{workspace === "admin" ? t("shell.adminConsole") : t("shell.userWorkspace")}</p>
                          <h1 className="text-2xl font-semibold tracking-normal">{titleKey ? t(titleKey) : title}</h1>
                        </div>
                        <div className="flex flex-wrap items-center gap-2">
                          <LanguageSwitch className="w-28" />
                          {showRBACChrome ? (
                            <>
                              <Badge variant="secondary">{t("common.permissions", { count: state.session.permissions?.length ?? 0 })}</Badge>
                              <Badge variant="outline">{t("common.scopes", { count: state.session.resource_scopes?.length ?? 0 })}</Badge>
                            </>
                          ) : null}
                        </div>
                      </div>
                    </header>
                    <div className="px-4 py-6 lg:px-8">{children}</div>
                  </section>
                </div>
              </main>
              <Toaster />
            </TooltipProvider>
          </ConsoleSessionContext.Provider>
        );
      }}
    </ConsoleSessionProvider>
  );
}

function DefaultWorkspaceRedirect({ appName, session }: { appName: string; session: ControlSession }) {
  const router = useRouter();

  useEffect(() => {
    router.replace(canUseAdminWorkspace(session) ? firstAccessibleAdminHref(session) : firstAccessibleUserHref(session));
  }, [router, session]);

  return <ConsoleLoading appName={appName} />;
}

function ConsoleSessionProvider({
  children,
  initialUser,
}: {
  children: (state: ConsoleSessionState) => ReactNode;
  initialUser: InitialUser;
}) {
  const { locale } = useI18n();
  const [session, setSession] = useState<ControlSession | null>(null);
  const [loading, setLoading] = useState(Boolean(initialUser));
  const [error, setError] = useState("");
  const [errorCode, setErrorCode] = useState("");

  async function refresh() {
    if (!initialUser) {
      return;
    }
    setLoading(true);
    setError("");
    setErrorCode("");
    try {
      const result = await controlGet<ControlSession>("/api/control/session");
      setSession(result);
    } catch (requestError) {
      setSession(null);
      setErrorCode(requestError instanceof ControlAPIError ? requestError.code : "");
      setError(localizeControlError(requestError, locale));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, [initialUser?.id]);

  return children({ error, errorCode, loading, refresh, session });
}

export function AuthScreen({ appName, registrationClosed }: { appName: string; registrationClosed: boolean }) {
  const { locale, t } = useI18n();
  const [mode, setMode] = useState<"sign-in" | "sign-up">("sign-in");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [errorDescription, setErrorDescription] = useState("");
  const [setupToken, setSetupToken] = useState("");

  useEffect(() => {
    if (registrationClosed && mode === "sign-up") {
      setMode("sign-in");
    }
  }, [mode, registrationClosed]);

  useEffect(() => {
    setSetupToken(new URLSearchParams(window.location.search).get("setup_token") ?? "");
  }, []);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    setErrorDescription("");
    const form = new FormData(event.currentTarget);
    const response = await fetch(`/api/auth/${mode}/email`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        ...(mode === "sign-up" && setupToken ? { "x-oss-setup-token": setupToken } : {}),
      },
      body: JSON.stringify({
        email: form.get("email"),
        password: form.get("password"),
        name: form.get("name") || String(form.get("email") ?? ""),
      }),
    });
    setSubmitting(false);
    if (!response.ok) {
      const authError = await readAuthError(response);
      if (authError) {
        setError(localizeControlError(authError, locale));
        return;
      }
      setError(t("auth.failedTitle"));
      setErrorDescription(t("auth.failedDescription"));
      return;
    }
    window.location.assign("/console");
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-background p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <div className="flex items-start justify-between gap-3">
            <CardTitle>{appName}</CardTitle>
            <LanguageSwitch className="w-28" />
          </div>
          <CardDescription>{mode === "sign-in" ? t("auth.signInDescription") : t("auth.signUpDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-5" onSubmit={submit}>
            {registrationClosed ? (
              <Alert>
                <AlertTitle>{t("auth.signupDisabledTitle")}</AlertTitle>
                <AlertDescription>{t("auth.signupDisabledDescription")}</AlertDescription>
              </Alert>
            ) : null}
            <FieldGroup>
              {mode === "sign-up" ? (
                <Field>
                  <FieldLabel htmlFor="name">{t("auth.name")}</FieldLabel>
                  <Input id="name" name="name" placeholder={t("auth.namePlaceholder")} />
                </Field>
              ) : null}
              <Field>
                <FieldLabel htmlFor="email">{t("auth.email")}</FieldLabel>
                <Input id="email" name="email" placeholder="you@example.com" required type="email" />
              </Field>
              <Field>
                <FieldLabel htmlFor="password">{t("auth.password")}</FieldLabel>
                <Input id="password" minLength={8} name="password" required type="password" />
              </Field>
            </FieldGroup>
            {error ? (
              <Alert variant="destructive">
                <AlertTitle>{error}</AlertTitle>
                {errorDescription ? <AlertDescription>{errorDescription}</AlertDescription> : null}
              </Alert>
            ) : null}
            <div className="flex items-center justify-between gap-3">
              <Button disabled={submitting} type="submit">
                <KeyRoundIcon data-icon="inline-start" />
                {mode === "sign-in" ? t("auth.signIn") : t("auth.signUp")}
              </Button>
              {mode === "sign-in" && !registrationClosed ? (
                <Button onClick={() => setMode("sign-up")} type="button" variant="ghost">
                  {t("auth.createAccount")}
                </Button>
              ) : null}
              {mode === "sign-up" ? (
                <Button onClick={() => setMode("sign-in")} type="button" variant="ghost">
                  {t("auth.useExistingAccount")}
                </Button>
              ) : null}
            </div>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}

async function readAuthError(response: Response): Promise<{ code: string; message?: string; details?: Record<string, unknown> } | null> {
  const body = (await response.json().catch(() => ({}))) as { error?: { code?: unknown; message?: unknown; details?: unknown } };
  if (!body.error || typeof body.error.code !== "string") {
    return null;
  }
  return {
    code: body.error.code,
    message: typeof body.error.message === "string" ? body.error.message : undefined,
    details: isRecord(body.error.details) ? body.error.details : undefined,
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function SetupScreen({
  appName,
  error,
  errorCode,
  initialUser,
  onReady,
}: {
  appName: string;
  error: string;
  errorCode: string;
  initialUser: NonNullable<InitialUser>;
  onReady: () => Promise<void>;
}) {
  const { locale, t } = useI18n();
  const [submitting, setSubmitting] = useState(false);
  const [localError, setLocalError] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setLocalError("");
    const form = new FormData(event.currentTarget);
    try {
      await controlPost<ControlSession>("/api/control/bootstrap", {
        organization_name: form.get("organization_name"),
        organization_slug: form.get("organization_slug"),
      });
      toast.success(t("setup.organizationCreated"));
      await onReady();
    } catch (requestError) {
      setLocalError(localizeControlError(requestError, locale));
    } finally {
      setSubmitting(false);
    }
  }

  if (errorCode === "OSS_OWNER_REQUIRED") {
    return (
      <main className="flex min-h-screen items-center justify-center bg-background p-6">
        <Card className="w-full max-w-md">
          <CardHeader>
            <div className="flex items-start justify-between gap-3">
              <CardTitle>{appName}</CardTitle>
              <LanguageSwitch className="w-28" />
            </div>
            <CardDescription>{initialUser.email}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            <Alert variant="destructive">
              <AlertTitle>{t("auth.ownerOnlyTitle")}</AlertTitle>
              <AlertDescription>{t("auth.ownerOnlyDescription")}</AlertDescription>
            </Alert>
            <SignOutButton compact={false} />
          </CardContent>
        </Card>
        <Toaster />
      </main>
    );
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-background p-6">
      <Card className="w-full max-w-xl">
        <CardHeader>
          <div className="flex items-start justify-between gap-3">
            <CardTitle>{appName}</CardTitle>
            <LanguageSwitch className="w-28" />
          </div>
          <CardDescription>{initialUser.email}</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-5" onSubmit={submit}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="organization_name">{t("setup.organizationName")}</FieldLabel>
                <Input id="organization_name" name="organization_name" placeholder={t("setup.organizationNamePlaceholder")} required />
              </Field>
              <Field>
                <FieldLabel htmlFor="organization_slug">{t("setup.organizationSlug")}</FieldLabel>
                <Input id="organization_slug" name="organization_slug" pattern="[a-z0-9][a-z0-9-]{1,61}[a-z0-9]" placeholder="acme-network" required />
                <FieldDescription>{t("setup.organizationSlugHelp")}</FieldDescription>
              </Field>
            </FieldGroup>
            {error || localError ? (
              <Alert variant="destructive">
                <AlertTitle>{localError || error}</AlertTitle>
              </Alert>
            ) : null}
            <Button disabled={submitting} type="submit">{t("setup.createOrganization")}</Button>
          </form>
        </CardContent>
      </Card>
      <Toaster />
    </main>
  );
}

function WorkspaceSwitch({ session, workspace }: { session: ControlSession; workspace: Workspace }) {
  const { t } = useI18n();
  const canAdmin = canUseAdminWorkspace(session);
  const adminHref = firstAccessibleAdminHref(session);
  const userHref = firstAccessibleUserHref(session);

  return (
    <Tabs value={workspace}>
      <TabsList className="grid w-full grid-cols-2">
        <TabsTrigger asChild value="user">
          <Link href={userHref}>{t("shell.user")}</Link>
        </TabsTrigger>
        {canAdmin ? (
          <TabsTrigger asChild value="admin">
            <Link href={adminHref}>{t("shell.admin")}</Link>
          </TabsTrigger>
        ) : (
          <Tooltip>
            <TooltipTrigger asChild>
              <TabsTrigger disabled value="admin">{t("shell.admin")}</TabsTrigger>
            </TooltipTrigger>
            <TooltipContent>{t("shell.adminPermissionRequired")}</TooltipContent>
          </Tooltip>
        )}
      </TabsList>
    </Tabs>
  );
}

function firstAccessibleAdminHref(session: ControlSession): string {
  return defaultConsoleRegistry.itemsByWorkspace.admin.find((item) => canAccessNavItem(session, item))?.href ?? "/console/admin/overview";
}

function firstAccessibleUserHref(session: ControlSession): string {
  return defaultConsoleRegistry.itemsByWorkspace.user.find((item) => canAccessNavItem(session, item))?.href ?? "/console/user/rules";
}

function canAccessNavItem(session: ControlSession | null, item: { permissions: string[]; requiredPermissions?: string[] }): boolean {
  return hasAnyPermission(session, item.permissions) && (item.requiredPermissions ?? []).every((permission) => hasAnyPermission(session, [permission]));
}

function hasConsoleCapability(registry: { capabilities: string[] }, capability: string): boolean {
  return registry.capabilities.includes(capability);
}

function NavItem({
  active,
  href,
  icon: Icon,
  label,
}: {
  active: boolean;
  href: string;
  icon: React.ComponentType<React.SVGProps<SVGSVGElement>>;
  label: string;
}) {
  const pathname = usePathname();
  const isActive = active || pathname === href;
  return (
    <Button asChild className="justify-start" variant={isActive ? "secondary" : "ghost"}>
      <Link href={href}>
        <Icon data-icon="inline-start" />
        {label}
      </Link>
    </Button>
  );
}

function SignOutButton({ compact = true }: { compact?: boolean }) {
  const { t } = useI18n();
  async function signOut() {
    await fetch("/api/auth/sign-out", { method: "POST" });
    window.location.assign("/");
  }

  return (
    <Button onClick={signOut} size={compact ? "icon-sm" : "default"} title={t("shell.signOut")} type="button" variant="ghost">
      <LogOutIcon />
      {compact ? <span className="sr-only">{t("shell.signOut")}</span> : t("shell.signOut")}
    </Button>
  );
}

function ConsoleLoading({ appName }: { appName: string }) {
  return (
    <main className="min-h-screen bg-background p-6">
      <div className="mx-auto flex max-w-6xl flex-col gap-4">
        <div className="flex items-center justify-between">
          <div className="flex flex-col gap-2">
            <Skeleton className="h-6 w-48" />
            <Skeleton className="h-4 w-64" />
          </div>
          <Badge variant="secondary">{appName}</Badge>
        </div>
        <Skeleton className="h-[560px] w-full" />
      </div>
    </main>
  );
}

function initials(value: string): string {
  return value
    .split("@")[0]
    .split(/[._-]/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase())
    .join("") || "U";
}
