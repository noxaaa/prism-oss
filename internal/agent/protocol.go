package agent

const (
	ProtocolMajor = 2
	ProtocolMinor = 0
)

type ProtocolVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

func CurrentProtocolVersion() ProtocolVersion {
	return ProtocolVersion{Major: ProtocolMajor, Minor: ProtocolMinor}
}

func (server ProtocolVersion) Accepts(agent ProtocolVersion) bool {
	if server.Major != agent.Major {
		return false
	}
	if agent.Minor > server.Minor {
		return false
	}
	return server.Minor-agent.Minor <= 1
}
