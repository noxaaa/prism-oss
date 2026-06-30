package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOSSWebCorePackageBuildContract(t *testing.T) {
	root := repoRoot(t)
	globals := readText(t, filepath.Join(root, "apps", "web", "src", "app", "globals.css"))
	if !strings.Contains(globals, `@source "../../../../packages/web-core/src"`) {
		t.Fatalf("OSS web app Tailwind sources must include packages/web-core/src")
	}

	nextConfig := readText(t, filepath.Join(root, "apps", "web", "next.config.mjs"))
	for _, required := range []string{
		`transpilePackages: ["@noxaaa/prism-oss-web-core"]`,
		`"@noxaaa/prism-oss-web-core/console/edition-registry$"`,
		`"@noxaaa/prism-oss-web-core/console/i18n-core$"`,
	} {
		if !strings.Contains(nextConfig, required) {
			t.Fatalf("OSS web app must preserve web-core build wiring; missing %q", required)
		}
	}
}
