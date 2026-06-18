package domain

import "testing"

func TestTrafficUsageTotalModeAddsUploadAndDownload(t *testing.T) {
	usage := TrafficUsage{UploadBytes: 100, DownloadBytes: 250}

	if got := usage.CountedBytes(TrafficLimitModeTotal); got != 350 {
		t.Fatalf("expected total 350, got %d", got)
	}
}

func TestTrafficUsageDirectionalModes(t *testing.T) {
	usage := TrafficUsage{UploadBytes: 100, DownloadBytes: 250}

	if got := usage.CountedBytes(TrafficLimitModeUploadOnly); got != 100 {
		t.Fatalf("expected upload only 100, got %d", got)
	}
	if got := usage.CountedBytes(TrafficLimitModeDownloadOnly); got != 250 {
		t.Fatalf("expected download only 250, got %d", got)
	}
}

func TestTrafficUsageMaxOfUpDownMode(t *testing.T) {
	usage := TrafficUsage{UploadBytes: 100, DownloadBytes: 250}

	if got := usage.CountedBytes(TrafficLimitModeMaxOfUpDown); got != 250 {
		t.Fatalf("expected max 250, got %d", got)
	}
}

func TestQuotaAllowsWarnOnlyOverLimitWithoutDisabling(t *testing.T) {
	quota := Quota{
		TrafficLimitBytes: 300,
		TrafficLimitMode:  TrafficLimitModeTotal,
		OverLimitAction:   OverLimitActionWarnOnly,
	}
	usage := TrafficUsage{UploadBytes: 100, DownloadBytes: 250}

	decision := quota.Evaluate(usage)

	if !decision.Exceeded {
		t.Fatalf("expected over limit")
	}
	if decision.DisableRules {
		t.Fatalf("warn-only quota must not disable rules")
	}
}
