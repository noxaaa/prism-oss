package service

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNyanpassRuleToPortableDoesNotCommitTargetRefsOnError(t *testing.T) {
	targetRefsByHostPort := map[string]string{}
	_, targets, _, err := nyanpassRuleToPortable(0, nyanpassRulePayload{
		Name:       "bad-mixed-dest",
		ListenPort: 443,
		Dest:       []string{"1.1.1.1:443", "missing-port"},
	}, targetRefsByHostPort)
	if err == nil {
		t.Fatal("expected invalid mixed-dest rule to fail")
	}
	if len(targets) != 0 || len(targetRefsByHostPort) != 0 {
		t.Fatalf("failed Nyanpass rule should not commit target refs, targets=%#v refs=%#v", targets, targetRefsByHostPort)
	}

	rule, targets, _, err := nyanpassRuleToPortable(1, nyanpassRulePayload{
		Name:       "valid-after-bad",
		ListenPort: 443,
		Dest:       []string{"1.1.1.1:443"},
	}, targetRefsByHostPort)
	if err != nil {
		t.Fatalf("expected later valid rule to import: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected later valid rule to create its missing target, got %#v", targets)
	}
	if rule.Upstream.TargetRef != targets[0].Ref {
		t.Fatalf("expected rule upstream to reference created target, rule=%#v targets=%#v", rule.Upstream, targets)
	}
}

func TestNyanpassImportSourceReturnsStructuredIssues(t *testing.T) {
	service := NewControlService(nil)
	result, err := service.nyanpassImportSource(`
{"dest":["1.1.1.1:443"],"listen_port":443,"name":"tls-rule","tls":{"sni":"example.com"}}
{"dest":["missing-port"],"listen_port":443,"name":"bad-dest"}
{"dest":["1.1.1.1:443"],"dest_policy":"round_robin","listen_port":443,"name":"bad-policy"}
{"accept_proxy_protocol":9,"dest":["1.1.1.1:443"],"listen_port":443,"name":"bad-proxy"}
`)
	if err != nil {
		t.Fatalf("expected invalid Nyanpass entries to produce per-rule issues, got %v", err)
	}
	if result.Skipped != 4 || len(result.Errors) != 4 {
		t.Fatalf("expected four skipped issues, got skipped=%d errors=%#v", result.Skipped, result.Errors)
	}
	assertImportIssue(t, result.Errors[0], "IMPORT_NYANPASS_TLS_UNSUPPORTED", "nyanpass", 0)
	assertImportIssue(t, result.Errors[1], "IMPORT_NYANPASS_INVALID_DEST", "nyanpass", 1)
	if result.Errors[1].Details["dest_index"] != 0 || result.Errors[1].Details["dest"] != "missing-port" {
		t.Fatalf("expected invalid dest details, got %#v", result.Errors[1].Details)
	}
	assertImportIssue(t, result.Errors[2], "IMPORT_NYANPASS_UNSUPPORTED_DEST_POLICY", "nyanpass", 2)
	if result.Errors[2].Details["actual"] != "round_robin" {
		t.Fatalf("expected unsupported dest_policy detail, got %#v", result.Errors[2].Details)
	}
	assertImportIssue(t, result.Errors[3], "IMPORT_NYANPASS_INVALID_ACCEPT_PROXY_PROTOCOL", "nyanpass", 3)
	if result.Errors[3].Details["actual"] != 9 {
		t.Fatalf("expected invalid proxy protocol detail, got %#v", result.Errors[3].Details)
	}
}

func TestBoundedImportNameTruncatesByUTF8Bytes(t *testing.T) {
	name := boundedImportName(strings.Repeat("界", 40), "-target-1", 120)
	if len(name) > 120 {
		t.Fatalf("expected generated name to fit 120 bytes, got %d bytes", len(name))
	}
	if !strings.HasSuffix(name, "-target-1") {
		t.Fatalf("expected generated name to preserve suffix, got %q", name)
	}
	if !utf8.ValidString(name) {
		t.Fatalf("expected generated name to preserve UTF-8 boundaries, got %q", name)
	}
}

func assertImportIssue(t *testing.T, issue RuleImportIssue, code string, scope string, index int) {
	t.Helper()
	if issue.Code != code || issue.Scope != scope || issue.Index == nil || *issue.Index != index {
		t.Fatalf("expected issue code=%s scope=%s index=%d, got %#v", code, scope, index, issue)
	}
}
