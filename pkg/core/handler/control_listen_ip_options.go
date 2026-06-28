package handler

import (
	"strconv"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func parseListenIPOptionPortSegments(port int, raw string) ([]repo.InboundBindingPortSegmentRecord, bool) {
	if strings.TrimSpace(raw) == "" {
		if port == 0 {
			return nil, true
		}
		return []repo.InboundBindingPortSegmentRecord{{StartPort: port, EndPort: port}}, true
	}
	if port != 0 {
		return nil, false
	}
	segments := make([]repo.InboundBindingPortSegmentRecord, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		startText := part
		endText := part
		if strings.Contains(part, "-") {
			pieces := strings.Split(part, "-")
			if len(pieces) != 2 {
				return nil, false
			}
			startText = strings.TrimSpace(pieces[0])
			endText = strings.TrimSpace(pieces[1])
		}
		startPort, err := strconv.Atoi(startText)
		if err != nil {
			return nil, false
		}
		endPort, err := strconv.Atoi(endText)
		if err != nil {
			return nil, false
		}
		if startPort < 1 || endPort < 1 || startPort > 65535 || endPort > 65535 {
			return nil, false
		}
		if startPort > endPort {
			startPort, endPort = endPort, startPort
		}
		segments = append(segments, repo.InboundBindingPortSegmentRecord{StartPort: startPort, EndPort: endPort})
	}
	return segments, len(segments) > 0
}
