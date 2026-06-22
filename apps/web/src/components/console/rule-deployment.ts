import type { RuleDeployment, RuleDeploymentNode } from "./types";

type Translate = (key: string, vars?: Record<string, number>) => string;

export function ruleDeploymentLabel(deployment: RuleDeployment | null | undefined, t: Translate): string {
  if (deployment?.status === "DISABLED") {
    return t("rules.deploymentDisabled");
  }
  if (!deployment || deployment.total === 0) {
    return t("rules.deploymentNoNodes", { total: 0 });
  }
  if (deployment.failed > 0) {
    return t("rules.deploymentFailed", { failed: deployment.failed, total: deployment.total });
  }
  if (deployment.pending > 0) {
    return t("rules.deploymentPending", { pending: deployment.pending, total: deployment.total });
  }
  return t("rules.deploymentApplied", { applied: deployment.applied, total: deployment.total });
}

export function ruleDeploymentTone(deployment: RuleDeployment | null | undefined): "default" | "secondary" | "destructive" {
  if (deployment?.failed && deployment.failed > 0) {
    return "destructive";
  }
  if (!deployment || deployment.pending > 0 || deployment.total === 0) {
    return "secondary";
  }
  return "default";
}

export function failedRuleDeploymentNodes(deployment: RuleDeployment | null | undefined): RuleDeploymentNode[] {
  return safeDeploymentNodes(deployment).filter((node) => node.status === "FAILED");
}

export function safeDeploymentNodes(deployment: RuleDeployment | null | undefined): RuleDeploymentNode[] {
  return Array.isArray(deployment?.nodes) ? deployment.nodes : [];
}

export function ruleDeploymentEndpoint(node: RuleDeploymentNode): string {
  if (!node.protocol && !node.listen_ip && !node.port) {
    return "";
  }
  return `${node.protocol || ""} ${node.listen_ip || ""}${node.port ? `:${node.port}` : ""}`.trim();
}
