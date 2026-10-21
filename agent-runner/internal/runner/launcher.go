package runner

import (
	"context"
)

type DryRunLauncher struct{}

func (DryRunLauncher) Launch(ctx context.Context, task Task, repoPath string, sessionID string) (LaunchResult, error) {
	select {
	case <-ctx.Done():
		return LaunchResult{ExitCode: -1}, ctx.Err()
	default:
		return LaunchResult{ExitCode: 0}, nil
	}
}
