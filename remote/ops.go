package remote

import (
	"fmt"
	"strings"
)

// ClusterNodes prints the K8s node status from the remote machine.
func (c *Client) ClusterNodes() (string, error) {
	return c.KubectlOnMaster("get nodes -o wide")
}

// CiliumStatus prints the Cilium status from lab-master-0.
func (c *Client) CiliumStatus() (string, error) {
	return c.MultipassExec(remoteMasterName(), "cilium status 2>&1")
}

// InstallCiliumCLI installs the Cilium CLI on lab-master-0.
func (c *Client) InstallCiliumCLI() error {
	script := strings.Join([]string{
		`CILIUM_CLI_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/cilium-cli/main/stable.txt)`,
		`curl -fsSL --retry 3 -o /tmp/cilium-linux-amd64.tar.gz \`,
		`  https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-amd64.tar.gz`,
		`sudo tar xzf /tmp/cilium-linux-amd64.tar.gz -C /usr/local/bin`,
		`rm /tmp/cilium-linux-amd64.tar.gz`,
		`cilium version --client`,
	}, " && ")
	out, err := c.MultipassExec(remoteMasterName(), script)
	if err != nil {
		return fmt.Errorf("install cilium cli: %w", err)
	}
	fmt.Println(out)
	return nil
}

// SudoRun runs a command with sudo on the remote machine.
func (c *Client) SudoRun(cmd string) (string, error) {
	return c.Run("sudo " + cmd)
}

// FirewalldAddTrustedInterface adds an interface to the firewalld trusted zone and reloads.
// Required for multipass bridge (mpqemubr0) to work correctly on Rocky Linux.
func (c *Client) FirewalldAddTrustedInterface(iface string) error {
	cmds := []string{
		fmt.Sprintf("sudo firewall-cmd --permanent --zone=trusted --add-interface=%s", iface),
		"sudo firewall-cmd --reload",
	}
	for _, cmd := range cmds {
		if _, err := c.Run(cmd); err != nil {
			return fmt.Errorf("firewalld: %w", err)
		}
	}
	return nil
}

// CheckPrereqs verifies all required tools are installed on the remote machine.
func (c *Client) CheckPrereqs() {
	tools := []struct{ label, cmd string }{
		{"go", "export PATH=$PATH:/usr/local/go/bin && go version"},
		{"dotnet", "export PATH=$PATH:/home/seoy/.dotnet && dotnet --version"},
		{"podman", "podman --version"},
		{"kubectl", "kubectl version --client -o yaml 2>/dev/null | grep gitVersion"},
		{"kind", "/opt/go/bin/kind --version"},
		{"multipass", "multipass --version"},
		{"tofu", "tofu --version | head -1"},
	}
	fmt.Println("=== Remote machine prereq check ===")
	for _, t := range tools {
		c.MustRun(t.label, t.cmd)
	}
}
