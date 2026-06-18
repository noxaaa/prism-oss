import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

const sourceRoot = fileURLToPath(new URL("./src", import.meta.url));
const fullEditionRegistry = fileURLToPath(new URL("./src/components/console/edition-registry-full.ts", import.meta.url));
const fullEditionI18n = fileURLToPath(new URL("./src/components/console/i18n-full.ts", import.meta.url));
const webEdition = process.env.NEXT_PUBLIC_PRISM_EDITION ?? "";
if (webEdition !== "" && webEdition !== "oss" && webEdition !== "full") {
  throw new Error(`Unsupported NEXT_PUBLIC_PRISM_EDITION: ${webEdition}`);
}
if (webEdition === "full" && (!existsSync(fullEditionRegistry) || !existsSync(fullEditionI18n))) {
  throw new Error("NEXT_PUBLIC_PRISM_EDITION=full is not available in this OSS source tree.");
}
const alias =
  webEdition === "full"
    ? [
        { find: /^@\/components\/console\/edition-registry$/, replacement: fullEditionRegistry },
        { find: /^@\/components\/console\/i18n-core$/, replacement: fullEditionI18n },
        { find: "@", replacement: sourceRoot },
      ]
    : [{ find: "@", replacement: sourceRoot }];

export default defineConfig({
  cacheDir: process.env.VITE_CACHE_DIR ?? "node_modules/.vite",
  esbuild: {
    jsx: "automatic",
    jsxImportSource: "react",
  },
  resolve: {
    alias,
  },
});
