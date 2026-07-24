package lab

import (
	"os/exec"
	"strings"
)

// DiscoverNodeOCIRuntime attempts to determine the OCI runtime (e.g. crun)
// and its version for a Kubernetes node, by matching the node name to an
// infra-lab-managed VM and reading it over SSH.
//
// It returns ok=false whenever the node isn't backed by a VM this run of
// infra-lab can see, or the VM is unreachable — capability checks must
// degrade to NOT_VERIFIED in that case rather than fail outright, since
// nodes on unmanaged/remote clusters are an expected, non-error case.
func DiscoverNodeOCIRuntime(root, nodeName string) (runtimeName, version string, ok bool) {
	vms, err := ListAllVMs(root)
	if err != nil {
		return "", "", false
	}
	for _, vm := range vms {
		if vm.Name != nodeName || vm.IPv4 == "" {
			continue
		}
		out, err := exec.Command(
			"ssh",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "BatchMode=yes",
			"-o", "ConnectTimeout=5",
			"ubuntu@"+vm.IPv4,
			"crun", "--version",
		).Output()
		if err != nil {
			return "", "", false
		}
		return parseCrunVersion(out)
	}
	return "", "", false
}

// parseCrunVersion parses the first line of `crun --version`, e.g.
// "crun version 1.28" or "crun version 1.28.0-1ubuntu...".
func parseCrunVersion(out []byte) (runtimeName, version string, ok bool) {
	line, _, _ := strings.Cut(string(out), "\n")
	fields := strings.Fields(line)
	for i, f := range fields {
		if f == "version" && i+1 < len(fields) {
			return "crun", fields[i+1], true
		}
	}
	if len(fields) > 0 {
		return "crun", "", true
	}
	return "", "", false
}
