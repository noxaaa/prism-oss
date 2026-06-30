"use client";

import { Alert, AlertDescription, AlertTitle } from "@noxaaa/prism-oss-web-core/ui/alert";
import { Badge } from "@noxaaa/prism-oss-web-core/ui/badge";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@noxaaa/prism-oss-web-core/ui/hover-card";
import { localizeEnum, useI18n } from "@noxaaa/prism-oss-web-core/console/i18n";
import { failedRuleDeploymentNodes, ruleDeploymentEndpoint, ruleDeploymentLabel, ruleDeploymentTone, safeDeploymentNodes } from "@noxaaa/prism-oss-web-core/console/rule-deployment";
import type { Rule } from "@noxaaa/prism-oss-web-core/console/types";

export function RuleDeploymentCell({ rule }: { rule: Rule }) {
  const { locale, t } = useI18n();
  const deployment = rule.deployment;
  const failedNodes = failedRuleDeploymentNodes(deployment);
  const nodes = safeDeploymentNodes(deployment);
  const label = ruleDeploymentLabel(deployment, t);
  const tone = ruleDeploymentTone(deployment);

  return (
    <HoverCard>
      <HoverCardTrigger asChild>
        <Badge className="cursor-default" variant={tone}>
          {label}
        </Badge>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-80">
        <div className="space-y-3">
          <div className="text-sm font-medium">{t("rules.deployment")}</div>
          {failedNodes.length > 0 ? (
            <div className="space-y-2">
              <div className="text-xs font-medium text-destructive">{t("rules.deploymentFailures")}</div>
              {failedNodes.map((node) => (
                <div className="rounded-md border border-destructive/20 bg-destructive/5 p-2 text-xs" key={node.node_id}>
                  <div className="font-medium">{node.node_name || node.node_id}</div>
                  <div className="text-muted-foreground">{ruleDeploymentEndpoint(node)}</div>
                  <div className="mt-1 text-muted-foreground">
                    {t("rules.deploymentDataplane", {
                      expected: node.expected_dataplane || "-",
                      actual: node.actual_dataplane || "-",
                    })}
                  </div>
                  {node.owner ? <div className="mt-1 break-words text-muted-foreground">{t("rules.deploymentOwner", { owner: node.owner })}</div> : null}
                  {node.drift_status ? <div className="mt-1 break-words text-muted-foreground">{t("rules.deploymentDrift", { status: node.drift_status })}</div> : null}
                  {node.external_resource ? <div className="mt-1 break-words text-muted-foreground">{node.external_resource}</div> : null}
                  <div className="mt-1 break-words">{node.error_code || localizeEnum(node.status, locale)}</div>
                  {node.error_message ? <div className="mt-1 break-words text-muted-foreground">{node.error_message}</div> : null}
                </div>
              ))}
            </div>
          ) : (
            <div className="space-y-1 text-xs text-muted-foreground">
              {nodes.map((node) => (
                <div className="flex justify-between gap-3" key={node.node_id}>
                  <span>{node.node_name || node.node_id}</span>
                  <span>{[node.actual_dataplane || node.expected_dataplane, localizeEnum(node.status, locale)].filter(Boolean).join(" · ")}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}

export function RuleDeploymentSummary({ rule }: { rule: Rule | null }) {
  const { t } = useI18n();
  if (!rule?.deployment) {
    return null;
  }
  const failedNodes = failedRuleDeploymentNodes(rule.deployment);
  return (
    <Alert variant={rule.deployment.failed > 0 ? "destructive" : "default"}>
      <AlertTitle className="flex items-center gap-2">
        {t("rules.deployment")}
        <Badge variant={ruleDeploymentTone(rule.deployment)}>{ruleDeploymentLabel(rule.deployment, t)}</Badge>
      </AlertTitle>
      {failedNodes.length > 0 ? (
        <AlertDescription className="space-y-2">
          {failedNodes.map((node) => (
            <div className="break-words" key={node.node_id}>
              <span className="font-medium">{node.node_name || node.node_id}</span>
              {ruleDeploymentEndpoint(node) ? ` ${ruleDeploymentEndpoint(node)}` : ""}
              {node.error_code ? ` ${node.error_code}` : ""}
              {node.actual_dataplane || node.expected_dataplane ? ` ${node.actual_dataplane || node.expected_dataplane}` : ""}
              {node.error_message ? ` ${node.error_message}` : ""}
            </div>
          ))}
        </AlertDescription>
      ) : null}
    </Alert>
  );
}
