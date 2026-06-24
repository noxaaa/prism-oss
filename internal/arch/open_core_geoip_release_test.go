package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOSSInstallerConfiguresGeoIPDatabaseWithoutBlockingInstall(t *testing.T) {
	root := repoRoot(t)
	installSource := readText(t, filepath.Join(root, "scripts", "install.sh"))
	composeSource := readText(t, filepath.Join(root, "docker-compose.yml"))
	for _, required := range []string{
		"--geoip-db-url",
		"--skip-geoip-download",
		"download_geoip_database",
		"geoip/dbip-country-lite.mmdb",
		"GEOIP_DB_PATH=/data/geoip/dbip-country-lite.mmdb",
	} {
		if !strings.Contains(installSource, required) {
			t.Fatalf("installer must configure optional GeoIP database; missing %q", required)
		}
	}
	if !strings.Contains(installSource, "GeoIP database download failed; continuing with unknown locations") {
		t.Fatalf("installer must continue when GeoIP download fails")
	}
	if !strings.Contains(installSource, "mkdir -p geoip\n  chmod 0755 geoip\n  if [ \"$skip_geoip_download\" = \"1\" ]; then") {
		t.Fatalf("installer must create a readable geoip directory before honoring --skip-geoip-download")
	}
	for _, required := range []string{
		"./geoip:/data/geoip:ro",
		"GEOIP_DB_PATH: ${GEOIP_DB_PATH:-/data/geoip/dbip-country-lite.mmdb}",
	} {
		if !strings.Contains(composeSource, required) {
			t.Fatalf("compose must mount GeoIP database for control-plane; missing %q", required)
		}
	}
}
