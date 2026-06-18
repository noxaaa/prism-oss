import { ConsoleGateway } from "@/components/console/shell";
import { getConsoleServerContext } from "@/lib/server-console";

export const dynamic = "force-dynamic";

export default async function ConsolePage() {
  const context = await getConsoleServerContext();
  return <ConsoleGateway appName={context.appName} initialLocale={context.locale} initialUser={context.initialUser} registrationClosed={context.registrationClosed} />;
}
