package arch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const maxSourceLines = 1000

func TestSourceFilesStayBelowLineLimit(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	sourceExts := map[string]bool{
		".go":  true,
		".ts":  true,
		".tsx": true,
		".css": true,
		".sql": true,
	}

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".next", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !sourceExts[filepath.Ext(path)] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if isGeneratedSource(string(data)) {
			return nil
		}
		lines := countLines(string(data))
		if lines > maxSourceLines {
			t.Fatalf("%s has %d lines; source files must stay at or below %d lines", filepath.ToSlash(path), lines, maxSourceLines)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseAccessIsLimitedToRepositoryAndMigrate(t *testing.T) {
	forEachGoFile(t, func(path string, content string) {
		if strings.Contains(path, "/pkg/core/repo/") || strings.Contains(path, "/cmd/migrate/") {
			return
		}
		forbidden := []string{"database/sql", "sql.DB", "sql.Tx", "SELECT ", "INSERT ", "UPDATE ", "DELETE FROM "}
		for _, token := range forbidden {
			if strings.Contains(content, token) {
				t.Fatalf("%s contains forbidden database token %q outside repository/migrate", path, token)
			}
		}
	})
}

func TestServicesDoNotImportTransportOrValidatorLayers(t *testing.T) {
	forEachGoFile(t, func(path string, content string) {
		if !strings.Contains(path, "/pkg/core/service/") {
			return
		}
		forbidden := []string{
			"\"net/http\"",
			"github.com/noxaaa/prism-oss/pkg/core/handler",
			"github.com/noxaaa/prism-oss/pkg/core/validator",
		}
		for _, token := range forbidden {
			if strings.Contains(content, token) {
				t.Fatalf("%s imports forbidden service-layer dependency %q", path, token)
			}
		}
	})
}

func TestAgentsDoNotImportControlPlaneOrInfrastructureAdapters(t *testing.T) {
	forEachGoFile(t, func(path string, content string) {
		if !strings.Contains(path, "/cmd/node-agent/") &&
			!strings.Contains(path, "/cmd/monitor-agent/") &&
			!strings.Contains(path, "/pkg/core/agent/") {
			return
		}
		forbidden := []string{
			"github.com/noxaaa/prism-oss/pkg/core/config",
			"github.com/noxaaa/prism-oss/internal/dns",
			"github.com/noxaaa/prism-oss/pkg/core/cache",
			"github.com/noxaaa/prism-oss/pkg/core/queue",
			"github.com/hibiken/asynq",
			"github.com/redis/",
			"DATABASE_DRIVER",
			"DATABASE_URL",
			"QUEUE_DRIVER",
			"QUEUE_REDIS_URL",
			"CACHE_DRIVER",
			"CACHE_REDIS_URL",
			"CONTROL_PLANE_INTERNAL_URL",
			"CONTROL_PLANE_INTERNAL_JWT_SECRET",
			"AGENT_TOKEN_SIGNING_SECRET",
			"DNS_SECRET_ENCRYPTION_KEY",
			"better-auth",
			"billing",
			"license",
			"commercial",
		}
		for _, token := range forbidden {
			if strings.Contains(content, token) {
				t.Fatalf("%s imports or references forbidden token %q", path, token)
			}
		}
	})
}

func forEachGoFile(t *testing.T, visit func(path string, content string)) {
	t.Helper()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", ".next":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		normalized := filepath.ToSlash(path)
		visit(normalized, string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func countLines(content string) int {
	if content == "" {
		return 0
	}
	lines := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		lines++
	}
	return lines
}

func isGeneratedSource(content string) bool {
	prefix := content
	if len(prefix) > 512 {
		prefix = prefix[:512]
	}
	return strings.Contains(prefix, "Code generated") || strings.Contains(prefix, "@generated")
}
