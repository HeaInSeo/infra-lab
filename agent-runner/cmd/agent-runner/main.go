package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/HeaInSeo/infra-lab/agent-runner/internal/runner"
)

func main() {
	var cfg runner.Config
	var repos repoFlags

	flag.StringVar(&cfg.Lane, "lane", "", "fixed runner lane: operator or guardrail")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "log intended actions without mutating task state or launching Codex")
	flag.BoolVar(&cfg.Once, "once", false, "run one poll cycle and exit")
	flag.DurationVar(&cfg.PollInterval, "poll-interval", 5*time.Minute, "polling interval")
	flag.DurationVar(&cfg.LeaseDuration, "lease-duration", 30*time.Minute, "claim lease duration")
	flag.DurationVar(&cfg.CheckpointFreshness, "checkpoint-freshness", 15*time.Minute, "fresh checkpoint threshold")
	flag.StringVar(&cfg.FakeNotionPath, "fake-notion", "", "path to fake Notion JSON file")
	flag.StringVar(&cfg.RunnerID, "runner-id", "", "stable runner identifier")
	flag.Var(&repos, "repo", "repository allowlist entry repo=path, repeatable")
	flag.Parse()

	cfg.RepoAllowlist = repos
	if cfg.RunnerID == "" {
		host, _ := os.Hostname()
		cfg.RunnerID = "agent-runner@" + host
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "agent-runner: %v\n", err)
		os.Exit(2)
	}

	adapter, err := runner.NewFakeNotionAdapterFromFile(cfg.FakeNotionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-runner: fake notion: %v\n", err)
		os.Exit(2)
	}

	logger := runner.NewJSONLogger(os.Stdout)
	r := runner.New(cfg, adapter, runner.GitInspector{}, runner.DryRunLauncher{}, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := r.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Event(ctx, runner.Event{State: runner.StateFailed, Type: "runner_error", Message: err.Error()})
		os.Exit(1)
	}
}

type repoFlags map[string]string

func (r *repoFlags) String() string {
	if r == nil {
		return ""
	}
	parts := make([]string, 0, len(*r))
	for repo, path := range *r {
		parts = append(parts, repo+"="+path)
	}
	return strings.Join(parts, ",")
}

func (r *repoFlags) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("expected repo=path")
	}
	if *r == nil {
		*r = make(map[string]string)
	}
	(*r)[parts[0]] = parts[1]
	return nil
}
