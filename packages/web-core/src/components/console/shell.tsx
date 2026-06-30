"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  ActivityIcon,
  DatabaseIcon,
  KeyRoundIcon,
  LogOutIcon,
  RouteIcon,
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
import { Alert, AlertDescription, AlertTitle } from "@noxaaa/prism-oss-web-core/ui/alert";
import { Avatar, AvatarFallback } from "@noxaaa/prism-oss-web-core/ui/avatar";
import { Badge } from "@noxaaa/prism-oss-web-core/ui/badge";
import { Button } from "@noxaaa/prism-oss-web-core/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@noxaaa/prism-oss-web-core/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@noxaaa/prism-oss-web-core/ui/field";
import { Input } from "@noxaaa/prism-oss-web-core/ui/input";
import { Separator } from "@noxaaa/prism-oss-web-core/ui/separator";
import { Skeleton } from "@noxaaa/prism-oss-web-core/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@noxaaa/prism-oss-web-core/ui/tabs";
import { Toaster } from "@noxaaa/prism-oss-web-core/ui/sonner";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@noxaaa/prism-oss-web-core/ui/tooltip";
import { ControlAPIError, controlGet, controlPost } from "@noxaaa/prism-oss-web-core/console/control-api";
import { defaultConsoleRegistry, type ConsoleNavItem, type ConsoleRegistry, type Workspace } from "@noxaaa/prism-oss-web-core/console/edition-registry";
import { CardTableSkeleton, FieldRequirementBadge, SummaryCard, SummaryGrid } from "@noxaaa/prism-oss-web-core/console/shared";
import { I18nProvider, LanguageSwitch, localizeControlError, useI18n, type Locale, type MessageKey, type MessagesByLocale } from "@noxaaa/prism-oss-web-core/console/i18n";
import { canUseAdminWorkspace, hasAnyPermission, roleSummary } from "@noxaaa/prism-oss-web-core/console/permissions";
import type { ControlSession, InitialUser } from "@noxaaa/prism-oss-web-core/console/types";

type ConsoleSessionState = {
  error: string;
  errorCode: string;
  loading: boolean;
  refresh: () => Promise<void>;
  session: ControlSession | null;
};

const ConsoleSessionContext = createContext<ConsoleSessionState | null>(null);
const ConsoleRegistryContext = createContext<ConsoleRegistry>(defaultConsoleRegistry);

export type ConsoleGatewayProps = {
  appName: string;
  initialLocale: Locale;
  initialUser: InitialUser;
  messages?: MessagesByLocale;
  registry?: ConsoleRegistry;
  registrationClosed: boolean;
};

export type ConsoleShellProps = {
  active: string;
  appName: string;
  children: ReactNode;
  initialUser: InitialUser;
  messages?: MessagesByLocale;
  registry?: ConsoleRegistry;
  title: string;
  titleKey?: MessageKey;
  workspace: Workspace;
  initialLocale: Locale;
  registrationClosed: boolean;
};

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
  messages,
  registry = defaultConsoleRegistry,
  registrationClosed,
}: ConsoleGatewayProps) {
  return (
    <I18nProvider initialLocale={initialLocale} messages={messages}>
      <ConsoleRegistryContext.Provider value={registry}>
        <ConsoleGatewayContent appName={appName} initialUser={initialUser} registrationClosed={registrationClosed} />
      </ConsoleRegistryContext.Provider>
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
  messages,
  registry = defaultConsoleRegistry,
  registrationClosed,
  title,
  titleKey,
  workspace,
}: ConsoleShellProps) {
  return (
    <I18nProvider initialLocale={initialLocale} messages={messages}>
      <ConsoleRegistryContext.Provider value={registry}>
        <ConsoleShellContent active={active} appName={appName} initialUser={initialUser} registrationClosed={registrationClosed} title={title} titleKey={titleKey} workspace={workspace}>
          {children}
        </ConsoleShellContent>
      </ConsoleRegistryContext.Provider>
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
  const registry = useContext(ConsoleRegistryContext);
  const showRBACChrome = hasConsoleCapability(registry, "rbac");
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
                        {registry.itemsByWorkspace[workspace]
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
  const registry = useContext(ConsoleRegistryContext);

  useEffect(() => {
    router.replace(canUseAdminWorkspace(session, registry) ? firstAccessibleAdminHref(session, registry) : firstAccessibleUserHref(session, registry));
  }, [registry, router, session]);

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
                  <FieldLabel htmlFor="name">{t("auth.name")}<FieldRequirementBadge required /></FieldLabel>
                  <Input id="name" name="name" placeholder={t("auth.namePlaceholder")} />
                </Field>
              ) : null}
              <Field>
                <FieldLabel htmlFor="email">{t("auth.email")}<FieldRequirementBadge required /></FieldLabel>
                <Input id="email" name="email" placeholder="you@example.com" required type="email" />
              </Field>
              <Field>
                <FieldLabel htmlFor="password">{t("auth.password")}<FieldRequirementBadge required /></FieldLabel>
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

export async function readAuthError(response: Response): Promise<{ code: string; message?: string; details?: Record<string, unknown> } | null> {
  const body = (await response.json().catch(() => ({}))) as unknown;
  return normalizeAuthError(isRecord(body) ? body.error : undefined) ?? normalizeAuthError(body);
}

function normalizeAuthError(value: unknown): { code: string; message?: string; details?: Record<string, unknown> } | null {
  if (!isRecord(value) || typeof value.code !== "string") {
    return null;
  }
  return {
    code: value.code,
    message: typeof value.message === "string" ? value.message : undefined,
    details: isRecord(value.details) ? value.details : undefined,
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
                <FieldLabel htmlFor="organization_name">{t("setup.organizationName")}<FieldRequirementBadge required /></FieldLabel>
                <Input id="organization_name" name="organization_name" placeholder={t("setup.organizationNamePlaceholder")} required />
              </Field>
              <Field>
                <FieldLabel htmlFor="organization_slug">{t("setup.organizationSlug")}<FieldRequirementBadge required /></FieldLabel>
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
  const registry = useContext(ConsoleRegistryContext);
  const canAdmin = canUseAdminWorkspace(session, registry);
  const adminHref = firstAccessibleAdminHref(session, registry);
  const userHref = firstAccessibleUserHref(session, registry);

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

function firstAccessibleAdminHref(session: ControlSession, registry: ConsoleRegistry): string {
  return registry.itemsByWorkspace.admin.find((item) => canAccessNavItem(session, item))?.href ?? "/console/admin/overview";
}

function firstAccessibleUserHref(session: ControlSession, registry: ConsoleRegistry): string {
  return registry.itemsByWorkspace.user.find((item) => canAccessNavItem(session, item))?.href ?? "/console/user/rules";
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
  const { locale, t } = useI18n();
  const [submitting, setSubmitting] = useState(false);

  async function signOut() {
    setSubmitting(true);
    try {
      const response = await fetch("/api/auth/sign-out", { credentials: "same-origin", method: "POST" });
      if (!response.ok) {
        const authError = await readAuthError(response);
        toast.error(authError ? localizeControlError(authError, locale) : t("error.requestFailed"));
        return;
      }
      window.location.assign("/");
    } catch (requestError) {
      toast.error(localizeControlError(requestError, locale));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Button disabled={submitting} onClick={signOut} size={compact ? "icon-sm" : "default"} title={t("shell.signOut")} type="button" variant="ghost">
      <LogOutIcon />
      {compact ? <span className="sr-only">{t("shell.signOut")}</span> : t("shell.signOut")}
    </Button>
  );
}

function ConsoleLoading({ appName }: { appName: string }) {
  return (
    <main className="min-h-screen bg-background text-foreground">
      <div className="grid min-h-screen lg:grid-cols-[240px_1fr]">
        <aside className="border-r bg-card">
          <div className="flex h-full flex-col gap-4 p-4">
            <div className="flex items-center gap-3">
              <div className="flex size-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
                <DatabaseIcon />
              </div>
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">{appName}</div>
                <Skeleton className="mt-1 h-3 w-28" />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-1 rounded-md bg-muted p-1">
              <Skeleton className="h-8 rounded-sm" />
              <Skeleton className="h-8 rounded-sm" />
            </div>
            <nav className="flex flex-col gap-1">
              {Array.from({ length: 6 }, (_, index) => (
                <Skeleton className="h-9 w-full" key={index} />
              ))}
            </nav>
            <div className="mt-auto flex flex-col gap-3">
              <Separator />
              <div className="flex items-center gap-3">
                <Skeleton className="size-8 rounded-full" />
                <div className="min-w-0 flex-1">
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="mt-1 h-3 w-20" />
                </div>
                <Skeleton className="size-8" />
              </div>
            </div>
          </div>
        </aside>
        <section className="min-w-0">
          <header className="border-b bg-background/95 px-4 py-4 lg:px-8">
            <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
              <div className="flex flex-col gap-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-8 w-48" />
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Skeleton className="h-9 w-28" />
                <Skeleton className="h-6 w-24 rounded-full" />
              </div>
            </div>
          </header>
          <div className="px-4 py-6 lg:px-8">
            <div className="flex flex-col gap-6">
              <SummaryGrid>
                <SummaryCard icon={<DatabaseIcon />} label={<Skeleton className="h-4 w-20" />} loading value={null} />
                <SummaryCard icon={<UsersIcon />} label={<Skeleton className="h-4 w-24" />} loading value={null} />
                <SummaryCard icon={<RouteIcon />} label={<Skeleton className="h-4 w-16" />} loading value={null} />
                <SummaryCard icon={<ActivityIcon />} label={<Skeleton className="h-4 w-20" />} loading value={null} />
              </SummaryGrid>
              <CardTableSkeleton
                columns={5}
                description={<Skeleton className="h-4 w-64" />}
                rows={5}
                title={<Skeleton className="h-6 w-40" />}
              />
            </div>
          </div>
        </section>
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
