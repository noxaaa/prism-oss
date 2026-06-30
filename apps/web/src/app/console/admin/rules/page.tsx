import { RulesPage } from "@noxaaa/prism-oss-web-core/console/features/rules";
import { ConsoleShell } from "@noxaaa/prism-oss-web-core/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function AdminRulesRoute() {
  const context = await getConsoleServerContext();
  return (
    <ConsoleShell active="rules" appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} title="Rules" titleKey="nav.rules" workspace="admin">
      <RulesPage mode="admin" />
    </ConsoleShell>
  );
}
