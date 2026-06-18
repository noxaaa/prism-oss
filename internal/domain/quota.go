package domain

type TrafficLimitMode string

const (
	TrafficLimitModeTotal        TrafficLimitMode = "TOTAL"
	TrafficLimitModeUploadOnly   TrafficLimitMode = "UPLOAD_ONLY"
	TrafficLimitModeDownloadOnly TrafficLimitMode = "DOWNLOAD_ONLY"
	TrafficLimitModeMaxOfUpDown  TrafficLimitMode = "MAX_OF_UP_DOWN"
)

type OverLimitAction string

const (
	OverLimitActionDisableRule OverLimitAction = "DISABLE_RULE"
	OverLimitActionWarnOnly    OverLimitAction = "WARN_ONLY"
)

type TrafficUsage struct {
	UploadBytes   int64
	DownloadBytes int64
}

func (usage TrafficUsage) CountedBytes(mode TrafficLimitMode) int64 {
	switch mode {
	case TrafficLimitModeUploadOnly:
		return usage.UploadBytes
	case TrafficLimitModeDownloadOnly:
		return usage.DownloadBytes
	case TrafficLimitModeMaxOfUpDown:
		if usage.UploadBytes > usage.DownloadBytes {
			return usage.UploadBytes
		}
		return usage.DownloadBytes
	default:
		return usage.UploadBytes + usage.DownloadBytes
	}
}

type Quota struct {
	TrafficLimitBytes int64
	TrafficLimitMode  TrafficLimitMode
	OverLimitAction   OverLimitAction
}

type QuotaDecision struct {
	Exceeded     bool
	DisableRules bool
	CountedBytes int64
}

func (quota Quota) Evaluate(usage TrafficUsage) QuotaDecision {
	counted := usage.CountedBytes(quota.TrafficLimitMode)
	exceeded := quota.TrafficLimitBytes > 0 && counted > quota.TrafficLimitBytes
	return QuotaDecision{
		Exceeded:     exceeded,
		DisableRules: exceeded && quota.OverLimitAction == OverLimitActionDisableRule,
		CountedBytes: counted,
	}
}
