import { SettingsPage } from "@noxaaa/prism-oss-web-core/console/features/settings";
import { ConsoleShell } from "@noxaaa/prism-oss-web-core/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function AdminSettingsRoute() {
  const context = await getConsoleServerContext();
  return (
    <ConsoleShell active="settings" appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} title="Settings" titleKey="nav.settings" workspace="admin">
      <SettingsPage />
    </ConsoleShell>
  );
}
