package main

import (
	"testing"

	"github.com/noxaaa/prism-oss/pkg/edition"
)

func TestOSSControlPlaneEditionAcceptsOSSAndUnset(t *testing.T) {
	for _, value := range []string{"", "oss"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv("PRISM_EDITION", value)
			provider, err := ossControlPlaneEdition()
			if err != nil {
				t.Fatalf("expected OSS control-plane edition %q to be accepted: %v", value, err)
			}
			if provider.Key() != edition.KeyOSS {
				t.Fatalf("expected OSS provider, got %s", provider.Key())
			}
		})
	}
}

func TestOSSControlPlaneEditionRejectsFullAndUnsupportedValues(t *testing.T) {
	for _, value := range []string{"full", "unsupported"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv("PRISM_EDITION", value)
			if _, err := ossControlPlaneEdition(); err == nil {
				t.Fatalf("expected OSS control-plane edition %q to be rejected", value)
			}
		})
	}
}
