import { UserUsagePage } from "@noxaaa/prism-oss-web-core/console/features/usage";
import { ConsoleShell } from "@noxaaa/prism-oss-web-core/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function UserUsageRoute() {
  const context = await getConsoleServerContext();
  return (
    <ConsoleShell active="usage" appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} title="Usage" titleKey="nav.usage" workspace="user">
      <UserUsagePage />
    </ConsoleShell>
  );
}
