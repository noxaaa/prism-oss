import { AdminOverviewPage } from "@noxaaa/prism-oss-web-core/console/features/overview";
import { ConsoleShell } from "@noxaaa/prism-oss-web-core/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function AdminOverviewRoute() {
  const context = await getConsoleServerContext();
  return (
    <ConsoleShell active="overview" appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} title="Overview" titleKey="nav.overview" workspace="admin">
      <AdminOverviewPage />
    </ConsoleShell>
  );
}
