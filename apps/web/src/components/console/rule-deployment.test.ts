import { describe, expect, it } from "vitest";
import { failedRuleDeploymentNodes, ruleDeploymentLabel, ruleDeploymentTone } from "./rule-deployment";
import type { RuleDeployment } from "./types";

describe("rule deployment helpers", () => {
  it("makes deployment failures explicit in the table label", () => {
    const deployment: RuleDeployment = {
      status: "DEPLOY_FAILED",
      total: 5,
      applied: 3,
      failed: 2,
      pending: 0,
      nodes: [
        { node_id: "node_1", node_name: "node-a", status: "APPLIED", updated_at: "2026-06-21T01:00:00Z" },
        {
          node_id: "node_2",
          node_name: "node-b",
          status: "FAILED",
          error_code: "LISTENER_BIND_FAILED",
          error_message: "listen tcp 0.0.0.0:443: bind: address already in use",
          protocol: "TCP",
          listen_ip: "0.0.0.0",
          port: 443,
          updated_at: "2026-06-21T01:00:05Z",
        },
      ],
    };

    expect(ruleDeploymentLabel(deployment, (key, vars) => `${key}:${JSON.stringify(vars)}`)).toBe(
      'rules.deploymentFailed:{"failed":2,"total":5}',
    );
    expect(ruleDeploymentTone(deployment)).toBe("destructive");
    expect(failedRuleDeploymentNodes(deployment).map((node) => node.node_name)).toEqual(["node-b"]);
  });

  it("distinguishes pending deployments from fully applied deployments", () => {
    expect(ruleDeploymentTone({ status: "PENDING", total: 2, applied: 1, failed: 0, pending: 1, nodes: [] })).toBe("secondary");
    expect(ruleDeploymentTone({ status: "APPLIED", total: 2, applied: 2, failed: 0, pending: 0, nodes: [] })).toBe("default");
  });

  it("labels disabled rules without fabricating pending deployments", () => {
    expect(ruleDeploymentLabel({ status: "DISABLED", total: 0, applied: 0, failed: 0, pending: 0, nodes: [] }, (key) => key)).toBe(
      "rules.deploymentDisabled",
    );
  });
});
