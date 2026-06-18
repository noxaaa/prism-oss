import { TargetsPage } from "@/components/console/features/targets";
import { ConsoleShell } from "@/components/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function AdminTargetsRoute() {
  const context = await getConsoleServerContext();
  return (
    <ConsoleShell active="targets" appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} title="Targets" titleKey="nav.targets" workspace="admin">
      <TargetsPage mode="admin" />
    </ConsoleShell>
  );
}
