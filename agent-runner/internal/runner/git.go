package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type GitInspector struct{}

func (GitInspector) DefaultHead(ctx context.Context, repoPath string) (string, error) {
	branch, err := git(ctx, repoPath, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	remote, err := git(ctx, repoPath, "config", "--get", "branch."+strings.TrimSpace(branch)+".remote")
	if err != nil || strings.TrimSpace(remote) == "." {
		remotes, listErr := git(ctx, repoPath, "remote")
		if listErr != nil {
			return "", listErr
		}
		fields := strings.Fields(remotes)
		if len(fields) != 1 {
			return "", fmt.Errorf("cannot determine default remote: configure the current branch remote (found %d remotes)", len(fields))
		}
		remote = fields[0]
	}
	remote = strings.TrimSpace(remote)
	defaultRef, err := git(ctx, repoPath, "symbolic-ref", "--quiet", "refs/remotes/"+remote+"/HEAD")
	if err != nil {
		return "", fmt.Errorf("cannot resolve default branch for remote %q: %w", remote, err)
	}
	out, err := git(ctx, repoPath, "rev-parse", "--verify", strings.TrimSpace(defaultRef))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (GitInspector) CurrentBranch(ctx context.Context, repoPath string) (string, error) {
	out, err := git(ctx, repoPath, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (GitInspector) IsClean(ctx context.Context, repoPath string) (bool, error) {
	out, err := git(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
