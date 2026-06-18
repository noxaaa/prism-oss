import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { I18nProvider } from "./i18n";
import { AuthScreen } from "./shell";

describe("console shell auth screen", () => {
  it("hides sign-up controls when OSS registration is closed", () => {
    const markup = renderToStaticMarkup(
      React.createElement(
        I18nProvider,
        { initialLocale: "en" },
        React.createElement(AuthScreen, { appName: "OSS Test Console", registrationClosed: true }),
      ),
    );

    expect(markup).toContain("This OSS instance has already been initialized.");
    expect(markup).toContain("Sign in");
    expect(markup).not.toContain("Create account");
    expect(markup).not.toContain("Sign up");
  });

  it("keeps sign-up controls available before OSS registration closes", () => {
    const markup = renderToStaticMarkup(
      React.createElement(
        I18nProvider,
        { initialLocale: "en" },
        React.createElement(AuthScreen, { appName: "OSS Test Console", registrationClosed: false }),
      ),
    );

    expect(markup).toContain("Create account");
  });
});
