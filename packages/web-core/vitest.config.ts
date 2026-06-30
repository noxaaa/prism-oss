import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "vitest/config";

const rootDir = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  resolve: {
    alias: {
      "@noxaaa/prism-oss-web-core/console/shared": path.resolve(
        rootDir,
        "src/components/console/shared/index.tsx",
      ),
      "@noxaaa/prism-oss-web-core/console": path.resolve(
        rootDir,
        "src/components/console",
      ),
      "@noxaaa/prism-oss-web-core/ui": path.resolve(rootDir, "src/components/ui"),
      "@noxaaa/prism-oss-web-core/lib": path.resolve(rootDir, "src/lib"),
    },
  },
  test: {
    environment: "node",
    globals: true,
  },
});
