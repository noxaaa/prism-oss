import { MonitorsPage } from "@noxaaa/prism-oss-web-core/console/features/monitors";
import { ConsoleShell } from "@noxaaa/prism-oss-web-core/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function AdminMonitorsRoute() {
  const context = await getConsoleServerContext();
  return (
    <ConsoleShell active="monitors" appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} title="Monitors" titleKey="nav.monitors" workspace="admin">
      <MonitorsPage />
    </ConsoleShell>
  );
}
