package service

import (
	"strings"
	"time"
)

func clampFutureObservedAt(observedAt string, serverNow string) string {
	observedAt = strings.TrimSpace(observedAt)
	serverNow = strings.TrimSpace(serverNow)
	if observedAt == "" {
		return serverNow
	}
	observedTime, err := time.Parse(time.RFC3339Nano, observedAt)
	if err != nil {
		return serverNow
	}
	serverTime, err := time.Parse(time.RFC3339Nano, serverNow)
	if err != nil {
		return observedAt
	}
	if observedTime.After(serverTime) {
		return serverNow
	}
	return observedAt
}
