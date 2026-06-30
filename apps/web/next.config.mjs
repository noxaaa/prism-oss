import { existsSync } from "node:fs";
import path from "node:path";

const webEdition = process.env.NEXT_PUBLIC_PRISM_EDITION ?? "";
if (webEdition !== "" && webEdition !== "oss" && webEdition !== "full") {
  throw new Error(`Unsupported NEXT_PUBLIC_PRISM_EDITION: ${webEdition}`);
}
const fullEditionRegistry = path.resolve("./src/components/console/edition-registry-full.ts");
const fullEditionI18n = path.resolve("./src/components/console/i18n-full.ts");
if (webEdition === "full" && (!existsSync(fullEditionRegistry) || !existsSync(fullEditionI18n))) {
  throw new Error("NEXT_PUBLIC_PRISM_EDITION=full is not available in this OSS source tree.");
}

const nextConfig = {
  allowedDevOrigins: ["127.0.0.1"],
  outputFileTracingRoot: path.resolve("../.."),
  serverExternalPackages: ["pg"],
  transpilePackages: ["@noxaaa/prism-oss-web-core"],
  webpack(config) {
    if (webEdition === "full") {
      config.resolve.alias = {
        ...(config.resolve.alias ?? {}),
        "@/components/console/edition-registry$": fullEditionRegistry,
        "@/components/console/i18n-core$": fullEditionI18n,
        "@noxaaa/prism-oss-web-core/console/edition-registry$": fullEditionRegistry,
        "@noxaaa/prism-oss-web-core/console/i18n-core$": fullEditionI18n,
      };
    }
    return config;
  },
};

export default nextConfig;
