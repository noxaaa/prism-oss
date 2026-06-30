import { DNSPage } from "@noxaaa/prism-oss-web-core/console/features/dns";
import { ConsoleShell } from "@noxaaa/prism-oss-web-core/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function AdminDNSRoute() {
  const context = await getConsoleServerContext();
  return (
    <ConsoleShell active="dns" appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} title="DNS" titleKey="nav.dns" workspace="admin">
      <DNSPage />
    </ConsoleShell>
  );
}
