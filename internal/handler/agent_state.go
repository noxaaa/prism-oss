package handler

import (
	"sync"
	"time"

	"github.com/noxaaa/prism-oss/internal/agent"
)

type AgentStateRegistry struct {
	mu         sync.RWMutex
	metrics    map[string]AgentMetricsState
	generation map[string]int64
	active     map[string]bool
}

type AgentMetricsState struct {
	OrganizationID string               `json:"organization_id"`
	AgentType      string               `json:"agent_type"`
	AgentID        string               `json:"agent_id"`
	Status         string               `json:"status"`
	LastSeenAt     string               `json:"last_seen_at"`
	Metrics        agent.MetricsPayload `json:"metrics"`
}

func NewAgentStateRegistry() *AgentStateRegistry {
	return &AgentStateRegistry{metrics: map[string]AgentMetricsState{}, generation: map[string]int64{}, active: map[string]bool{}}
}

func (registry *AgentStateRegistry) MarkConnected(organizationID string, agentType string, agentID string) int64 {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	key := registry.key(organizationID, agentType, agentID)
	nextGeneration := registry.generation[key] + 1
	registry.generation[key] = nextGeneration
	registry.active[key] = true
	state := registry.metrics[key]
	state.OrganizationID = organizationID
	state.AgentType = agentType
	state.AgentID = agentID
	state.Status = "ONLINE"
	state.LastSeenAt = time.Now().UTC().Format(time.RFC3339Nano)
	state.Metrics = agent.MetricsPayload{}
	registry.metrics[key] = state
	return nextGeneration
}

func (registry *AgentStateRegistry) MarkDisconnected(organizationID string, agentType string, agentID string, generation int64) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	key := registry.key(organizationID, agentType, agentID)
	if registry.generation[key] != generation || !registry.active[key] {
		return false
	}
	registry.active[key] = false
	state := registry.metrics[key]
	state.OrganizationID = organizationID
	state.AgentType = agentType
	state.AgentID = agentID
	state.Status = "OFFLINE"
	state.LastSeenAt = time.Now().UTC().Format(time.RFC3339Nano)
	registry.metrics[key] = state
	return true
}

func (registry *AgentStateRegistry) IsCurrent(organizationID string, agentType string, agentID string, generation int64) bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	key := registry.key(organizationID, agentType, agentID)
	return registry.generation[key] == generation && registry.active[key]
}

func (registry *AgentStateRegistry) UpdateMetrics(organizationID string, agentType string, agentID string, metrics agent.MetricsPayload) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.updateMetricsLocked(organizationID, agentType, agentID, metrics)
}

func (registry *AgentStateRegistry) UpdateMetricsForConnection(organizationID string, agentType string, agentID string, generation int64, metrics agent.MetricsPayload) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	key := registry.key(organizationID, agentType, agentID)
	if registry.generation[key] != generation || !registry.active[key] {
		return false
	}
	registry.updateMetricsLocked(organizationID, agentType, agentID, metrics)
	return true
}

func (registry *AgentStateRegistry) updateMetricsLocked(organizationID string, agentType string, agentID string, metrics agent.MetricsPayload) {
	state := AgentMetricsState{
		OrganizationID: organizationID,
		AgentType:      agentType,
		AgentID:        agentID,
		Status:         "ONLINE",
		LastSeenAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Metrics:        metrics,
	}
	registry.metrics[registry.key(organizationID, agentType, agentID)] = state
}

func (registry *AgentStateRegistry) Latest(organizationID string, agentType string, agentID string) (AgentMetricsState, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	state, ok := registry.metrics[registry.key(organizationID, agentType, agentID)]
	return state, ok
}

func (registry *AgentStateRegistry) LatestByOrganizationAndType(organizationID string, agentType string) []AgentMetricsState {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]AgentMetricsState, 0)
	for _, state := range registry.metrics {
		if state.OrganizationID == organizationID && state.AgentType == agentType {
			result = append(result, state)
		}
	}
	return result
}

func (registry *AgentStateRegistry) key(organizationID string, agentType string, agentID string) string {
	return organizationID + "\x00" + agentType + "\x00" + agentID
}
