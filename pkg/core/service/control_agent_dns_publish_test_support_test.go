package service

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (repository healthDNSTestNodeRepository) DisableAutoNodeDNSPublishAddresses(_ context.Context, organizationID string, nodeID string, now string) error {
	for nodeIndex := range repository.store.nodes {
		node := &repository.store.nodes[nodeIndex]
		if node.OrganizationID != organizationID || node.ID != nodeID || node.DeletedAt != "" {
			continue
		}
		for addressIndex := range node.DNSPublishAddresses {
			candidate := &node.DNSPublishAddresses[addressIndex]
			if candidate.Source == "AUTO" && candidate.Enabled {
				candidate.Enabled = false
				candidate.UpdatedAt = now
			}
		}
		return nil
	}
	return repo.ErrNotFound
}
