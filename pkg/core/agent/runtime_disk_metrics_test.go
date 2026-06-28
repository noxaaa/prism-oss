package agent

import (
	"errors"
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
)

func TestNodeRuntimeMetricsReportsRootDiskUsageAndToleratesFailure(t *testing.T) {
	originalDiskUsage := diskUsage
	t.Cleanup(func() { diskUsage = originalDiskUsage })
	diskUsage = func(path string) (*disk.UsageStat, error) {
		if path != "/" {
			t.Fatalf("expected root disk usage path, got %q", path)
		}
		return &disk.UsageStat{Used: 100, Total: 200}, nil
	}
	runtime := NewNodeRuntime(RuntimeConfig{}, nil)
	metrics := runtime.collectMetrics()
	if metrics.DiskUsedBytes != 100 || metrics.DiskTotalBytes != 200 {
		t.Fatalf("expected disk usage in metrics, got %#v", metrics)
	}

	diskUsage = func(string) (*disk.UsageStat, error) {
		return nil, errors.New("disk unavailable")
	}
	metrics = runtime.collectMetrics()
	if metrics.DiskUsedBytes != 0 || metrics.DiskTotalBytes != 0 {
		t.Fatalf("expected failed disk collection to leave zero values, got %#v", metrics)
	}
	if metrics.Architecture == "" {
		t.Fatalf("expected metrics collection to continue after disk failure")
	}
}
