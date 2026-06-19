package auth

import "context"

type Feature string

const (
	FeatureForwardingRules Feature = "forwarding_rules"
	FeatureOfflineLicense  Feature = "offline_license"
)

type FeatureRequest struct {
	OrganizationID string
	MemberID       string
	Feature        Feature
}

type FeatureGate interface {
	Allow(ctx context.Context, request FeatureRequest) (bool, error)
}

type BasicFeatureGate struct{}

func (gate BasicFeatureGate) Allow(ctx context.Context, request FeatureRequest) (bool, error) {
	switch request.Feature {
	case FeatureForwardingRules:
		return true, nil
	default:
		return false, nil
	}
}
