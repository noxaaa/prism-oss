package auth

import (
	"context"
	"testing"
)

func TestBasicFeatureGateAllowsCoreFeatures(t *testing.T) {
	gate := BasicFeatureGate{}

	allowed, err := gate.Allow(context.Background(), FeatureRequest{
		OrganizationID: "org_1",
		Feature:        FeatureForwardingRules,
	})
	if err != nil {
		t.Fatalf("feature gate returned error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected core feature to be allowed")
	}
}

func TestBasicFeatureGateDeniesCommercialFeatures(t *testing.T) {
	gate := BasicFeatureGate{}

	allowed, err := gate.Allow(context.Background(), FeatureRequest{
		OrganizationID: "org_1",
		Feature:        FeatureOfflineLicense,
	})
	if err != nil {
		t.Fatalf("feature gate returned error: %v", err)
	}
	if allowed {
		t.Fatalf("expected commercial feature to be denied by basic gate")
	}
}
