package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOSSReleaseNodeAgentPackagesManagedHAProxy(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	for _, required := range []string{
		"haproxy:3.0-alpine",
		"docker cp \"$container:/usr/local/sbin/haproxy\" \"$out_dir/dataplane/haproxy/haproxy.bin\"",
		"docker cp \"$container:/lib\" \"$out_dir/dataplane/haproxy/rootfs/lib\"",
		"docker cp \"$container:/usr/lib\" \"$out_dir/dataplane/haproxy/rootfs/usr/lib\"",
		"ld-musl-*.so.1",
		"tar -C \"$out_dir\" -czf \"$assets_dir/node-agent-linux-${arch}.tar.gz\" node-agent dataplane",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("release workflow must package Prism-managed HAProxy with node-agent; missing %q", required)
		}
	}
}
