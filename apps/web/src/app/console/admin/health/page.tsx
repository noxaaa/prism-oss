import { HealthChecksPage } from "@/components/console/features/monitors";
import { ConsoleShell } from "@/components/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function AdminHealthRoute() {
  const context = await getConsoleServerContext();
  return (
    <ConsoleShell active="health" appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} title="Health Checks" titleKey="nav.health" workspace="admin">
      <HealthChecksPage />
    </ConsoleShell>
  );
}
