import { describe, expect, it } from "vitest";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { ControlAPIError } from "./control-api";
import { I18nProvider, localizeControlError, localizeImportIssue, resolveLocale, useI18n } from "./i18n";
import type { RuleImportIssue } from "./types";

describe("console i18n", () => {
  it("resolves explicit and browser locales to supported console locales", () => {
    expect(resolveLocale("en", ["zh-CN"])).toBe("en");
    expect(resolveLocale(null, ["zh-HK", "en-CA"])).toBe("zh-CN");
    expect(resolveLocale(null, ["fr-CA"])).toBe("zh-CN");
  });

  it("uses the server-provided initial locale for first render", () => {
    function Probe() {
      const { t } = useI18n();
      return React.createElement("span", null, t("auth.signInDescription"));
    }

    const markup = renderToStaticMarkup(
      React.createElement(I18nProvider, { initialLocale: "en" }, React.createElement(Probe)),
    );

    expect(markup).toContain("Sign in with email and password.");
    expect(markup).not.toContain("使用邮箱和密码登录。");
  });

  it("localizes control API errors without rendering backend English messages", () => {
    const error = new ControlAPIError(400, "VALIDATION_FAILED", "Import format is required.", {
      field: "format",
      supported_formats: ["PORTABLE_EXPORT", "NYANPASS"],
    });

    const message = localizeControlError(error, "zh-CN");

    expect(message).toContain("导入格式");
    expect(message).toContain("便携导出");
    expect(message).toContain("Nyanpass");
    expect(message).not.toContain("Import format is required.");
    expect(message).not.toContain("supported_formats");
  });

  it("localizes OSS single-user auth errors without rendering backend English messages", () => {
    const signupError = new ControlAPIError(403, "OSS_SIGNUP_DISABLED", "OSS registration is closed.");
    const setupTokenError = new ControlAPIError(403, "OSS_SETUP_TOKEN_REQUIRED", "A valid OSS setup token is required.");
    const ownerError = new ControlAPIError(403, "OSS_OWNER_REQUIRED", "Only the owner can access this OSS instance.");

    expect(localizeControlError(signupError, "zh-CN")).toBe("此 OSS 实例已完成初始化，不能再注册新账号。");
    expect(localizeControlError(setupTokenError, "zh-CN")).toBe("请使用安装器打印的 setup URL 创建第一个 owner 账号。");
    expect(localizeControlError(ownerError, "zh-CN")).toBe("此 OSS 实例仅允许初始化时创建的 owner 登录。");
    expect(localizeControlError(signupError, "zh-CN")).not.toContain("OSS registration is closed.");
    expect(localizeControlError(setupTokenError, "zh-CN")).not.toContain("A valid OSS setup token is required.");
    expect(localizeControlError(ownerError, "zh-CN")).not.toContain("Only the owner can access this OSS instance.");
  });

  it("localizes BetterAuth origin errors for reverse proxy misconfiguration", () => {
    const originError = new ControlAPIError(403, "INVALID_ORIGIN", "Invalid origin");
    const missingOriginError = new ControlAPIError(403, "MISSING_OR_NULL_ORIGIN", "Missing or null Origin");

    expect(localizeControlError(originError, "zh-CN")).toContain("浏览器来源不在认证可信列表中");
    expect(localizeControlError(missingOriginError, "zh-CN")).toContain("认证请求缺少浏览器来源");
    expect(localizeControlError(originError, "zh-CN")).not.toContain("Invalid origin");
    expect(localizeControlError(missingOriginError, "zh-CN")).not.toContain("Missing or null Origin");
  });

  it("localizes structured import issues with row context and translated reason", () => {
    const issue: RuleImportIssue = {
      code: "IMPORT_NYANPASS_TLS_UNSUPPORTED",
      scope: "nyanpass",
      index: 2,
      details: { format: "NYANPASS" },
    };

    expect(localizeImportIssue(issue, "zh-CN")).toBe("第 3 条 Nyanpass 规则未导入：当前运行态不支持 Nyanpass TLS/origin fetch 语义。");
  });

  it("prefers detailed import reason codes over generic import issue codes", () => {
    const issue: RuleImportIssue = {
      code: "IMPORT_RULE_DISABLED",
      scope: "rules",
      index: 0,
      details: { reason_code: "RULE_PORT_CONFLICT", port: 443 },
    };

    const message = localizeImportIssue(issue, "zh-CN");

    expect(message).toContain("端口已被占用");
    expect(message).not.toContain("规则与所选入口或已有开启规则冲突");
  });
});
