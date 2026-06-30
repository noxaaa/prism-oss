"use client";

import { HoverCard, HoverCardContent, HoverCardTrigger } from "@noxaaa/prism-oss-web-core/ui/hover-card";
import { localizeEnum, useI18n } from "@noxaaa/prism-oss-web-core/console/i18n";
import type { Rule } from "@noxaaa/prism-oss-web-core/console/types";

export function RuleMatchCell({ rule }: { rule: Rule }) {
  const { locale, t } = useI18n();
  const label = localizeEnum(rule.match.type, locale);
  const hostname = rule.match.sni_hostname?.trim();
  if (!hostname) {
    return <span>{label}</span>;
  }
  return (
    <HoverCard>
      <HoverCardTrigger asChild>
        <button className="text-left underline-offset-4 hover:underline" type="button">{label}</button>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-80">
        <div className="grid gap-1">
          <span className="text-xs text-muted-foreground">{t("rules.sniHostname")}</span>
          <span className="break-all font-mono text-xs">{hostname}</span>
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}
