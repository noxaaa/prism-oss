import { NodesPage } from "@/components/console/features/nodes";
import { ConsoleShell } from "@/components/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function AdminNodesRoute() {
  const context = await getConsoleServerContext();
  return (
    <ConsoleShell active="nodes" appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} title="Nodes" titleKey="nav.nodes" workspace="admin">
      <NodesPage mode="admin" />
    </ConsoleShell>
  );
}
