// Package remote provides SSH utilities for remote management of the seoy lab machine.
//
// Target machine: 100.123.80.48 (seoy / Rocky Linux 8.10)
// All multipass-based K8s VM operations are performed on this machine.
// Use REMOTE_HOST, REMOTE_USER, REMOTE_PASS environment variables to override defaults.
package remote

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	DefaultHost          = "100.123.80.48"
	DefaultUser          = "seoy"
	DefaultPort          = "22"
	DefaultClusterPrefix = "lab"
	DefaultRepoPath      = "/opt/go/src/github.com/HeaInSeo/infra-lab"
)

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

// Client wraps an SSH connection to the remote lab machine.
type Client struct {
	conn *ssh.Client
}

// Config holds SSH connection parameters.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
}

// DefaultConfig returns connection config from environment variables,
// falling back to the seoy lab machine defaults.
func DefaultConfig() Config {
	host := envOrDefault("REMOTE_HOST", DefaultHost)
	user := envOrDefault("REMOTE_USER", DefaultUser)
	pass := os.Getenv("REMOTE_PASS")
	port := envOrDefault("REMOTE_PORT", DefaultPort)
	return Config{Host: host, Port: port, User: user, Password: pass}
}

func remoteNamePrefix() string {
	return envOrDefault("REMOTE_NAME_PREFIX", DefaultClusterPrefix)
}

func remoteRepoPath() string {
	return envOrDefault("REMOTE_REPO_PATH", DefaultRepoPath)
}

func remoteMasterName() string {
	return remoteNamePrefix() + "-master-0"
}

// Dial opens an SSH connection using the given Config.
func Dial(cfg Config) (*Client, error) {
	sshCfg := &ssh.ClientConfig{
		User: cfg.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(cfg.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // lab-only machine
	}
	conn, err := ssh.Dial("tcp", cfg.Host+":"+cfg.Port, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", cfg.Host, err)
	}
	return &Client{conn: conn}, nil
}

// Close closes the underlying SSH connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Run executes a single command and returns trimmed stdout+stderr output.
// Returns an error if the command exits non-zero.
func (c *Client) Run(cmd string) (string, error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer sess.Close()

	out, err := sess.CombinedOutput(cmd)
	result := strings.TrimSpace(string(out))
	if err != nil {
		return result, fmt.Errorf("cmd %q: %w\n%s", cmd, err, result)
	}
	return result, nil
}

// MustRun executes a command and prints the result.
// Prints [FAIL] on error but does not panic.
func (c *Client) MustRun(label, cmd string) string {
	out, err := c.Run(cmd)
	if err != nil {
		fmt.Printf("[%s] FAIL: %v\n", label, err)
		return ""
	}
	if out != "" {
		fmt.Printf("[%s] %s\n", label, out)
	} else {
		fmt.Printf("[%s] OK\n", label)
	}
	return out
}

// MultipassExec runs a command inside a named multipass VM via `multipass exec`.
func (c *Client) MultipassExec(vmName, cmd string) (string, error) {
	return c.Run(fmt.Sprintf("multipass exec %s -- bash -lc %q", vmName, cmd))
}

// MultipassList returns the output of `multipass list`.
func (c *Client) MultipassList() (string, error) {
	return c.Run("multipass list")
}

// KubectlOnMaster runs a kubectl command on lab-master-0 using the exported kubeconfig.
// kubeconfig is assumed to be at the default infra-lab path on the remote machine.
func (c *Client) KubectlOnMaster(args string) (string, error) {
	kubeconfig := "KUBECONFIG=" + remoteRepoPath() + "/kubeconfig"
	return c.Run(fmt.Sprintf("%s kubectl %s", kubeconfig, args))
}
