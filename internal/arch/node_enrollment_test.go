package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeEnrollmentMigrationIndexesPendingCredentialLookup(t *testing.T) {
	root := repoRoot(t)
	migration := readText(t, filepath.Join(root, "migrations", "core", "00013_node_enrollment_profiles.sql"))
	required := []string{
		"CREATE INDEX agent_credentials_pending_enrollment_profile_idx",
		"ON agent_credentials(organization_id, enrollment_profile_id, created_at, id)",
		"WHERE enrollment_profile_id IS NOT NULL",
		"AND activated_at IS NULL",
		"AND revoked_at IS NULL",
		"DROP INDEX IF EXISTS agent_credentials_pending_enrollment_profile_idx",
	}
	for _, value := range required {
		if !strings.Contains(migration, value) {
			t.Fatalf("node enrollment migration must index pending enrollment credential lookup; missing %q", value)
		}
	}
}
