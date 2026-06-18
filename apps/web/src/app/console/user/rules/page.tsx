import { RulesPage } from "@/components/console/features/rules";
import { ConsoleShell } from "@/components/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function UserRulesRoute() {
  const context = await getConsoleServerContext();
  return (
    <ConsoleShell active="rules" appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} title="My forwarding rules" titleKey="page.myForwardingRules" workspace="user">
      <RulesPage mode="user" />
    </ConsoleShell>
  );
}
