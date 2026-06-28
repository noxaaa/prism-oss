package service

import "github.com/noxaaa/prism-oss/pkg/core/repo"

func toNodeSendIPRecords(inputs []NodeSendIPInput) []repo.NodeSendIPRecord {
	records := make([]repo.NodeSendIPRecord, 0, len(inputs))
	for _, input := range inputs {
		records = append(records, repo.NodeSendIPRecord{SendIP: input.SendIP, DisplayName: input.DisplayName, Enabled: true})
	}
	return records
}

func toNodeSendIPPayloads(sendIPs []repo.NodeSendIPRecord) []NodeSendIPPayload {
	payloads := make([]NodeSendIPPayload, 0, len(sendIPs))
	for _, sendIP := range sendIPs {
		payloads = append(payloads, NodeSendIPPayload{ID: sendIP.ID, SendIP: sendIP.SendIP, DisplayName: sendIP.DisplayName, Enabled: sendIP.Enabled})
	}
	return payloads
}

func defaultMaxRulePorts(value int) int {
	if value <= 0 {
		return 256
	}
	return value
}
