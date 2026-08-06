package runner

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Runner struct {
	cfg      Config
	notion   NotionAdapter
	git      GitInspector
	launcher Launcher
	log      *JSONLogger
}

func New(cfg Config, notion NotionAdapter, git GitInspector, launcher Launcher, log *JSONLogger) *Runner {
	return &Runner{cfg: cfg, notion: notion, git: git, launcher: launcher, log: log}
}

func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	for {
		if err := r.cycle(ctx); err != nil {
			r.log.Event(ctx, Event{Type: "cycle_error", State: StateFailed, Message: err.Error()})
		}
		if r.cfg.Once {
			return nil
		}
		select {
		case <-ctx.Done():
			r.log.Event(ctx, Event{Type: "shutdown", State: StateReleased, Message: ctx.Err().Error()})
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Runner) cycle(ctx context.Context) error {
	r.log.Event(ctx, Event{Type: "poll", State: StateIdle, Fields: map[string]any{"lane": r.cfg.Lane, "dry_run": r.cfg.DryRun}})
	tasks, err := r.notion.ListCandidates(ctx, r.cfg.Lane)
	if err != nil {
		return err
	}
	now := r.cfg.now()
	for _, task := range tasks {
		decision := r.evaluate(ctx, task, now)
		if !decision.Runnable {
			r.log.Event(ctx, Event{Type: "skip", State: decision.State, TaskID: task.ID, Message: decision.Reason})
			continue
		}
		return r.runTask(ctx, task, decision.RepoPath, now)
	}
	return nil
}

type decision struct {
	Runnable bool
	State    string
	Reason   string
	RepoPath string
}

func (r *Runner) evaluate(ctx context.Context, task Task, now time.Time) decision {
	if !isRunnableStatus(task.Status) {
		return decision{State: StateIdle, Reason: "status is not runnable"}
	}
	if task.SessionID != "" && task.LeaseUntil != nil && task.LeaseUntil.After(now) {
		return decision{State: StateIdle, Reason: "valid session lease exists"}
	}
	if task.SessionID != "" && task.LeaseUntil != nil && task.LeaseUntil.Before(now) {
		if task.CheckpointAt != nil && now.Sub(*task.CheckpointAt) < r.cfg.CheckpointFreshness {
			return decision{State: StateCheckpointStale, Reason: "lease expired but checkpoint is fresh"}
		}
		return decision{State: StateReleased, Reason: "expired lease takeover is not implemented in phase 1"}
	}
	if task.Repository == "" || task.DefaultHeadSHA == "" || task.TargetBranch == "" || task.PR == "" {
		return decision{State: StateFailed, Reason: "missing repository/default_head_sha/target_branch/pr"}
	}
	if deniedCategory(task.WorkCategory) {
		return decision{State: StateFailed, Reason: "work category is denied for automatic execution"}
	}
	repoPath, ok := r.cfg.RepoAllowlist[task.Repository]
	if !ok {
		return decision{State: StateFailed, Reason: "repository is not allowlisted"}
	}
	head, err := r.git.DefaultHead(ctx, repoPath)
	if err != nil {
		return decision{State: StateFailed, Reason: "cannot read default head: " + err.Error()}
	}
	if head != task.DefaultHeadSHA {
		return decision{State: StateFailed, Reason: fmt.Sprintf("default head mismatch: row=%s local=%s", task.DefaultHeadSHA, head)}
	}
	if r.cfg.Lane == LaneOperator {
		branch, err := r.git.CurrentBranch(ctx, repoPath)
		if err != nil {
			return decision{State: StateFailed, Reason: "cannot read current branch: " + err.Error()}
		}
		if branch != task.TargetBranch {
			return decision{State: StateFailed, Reason: fmt.Sprintf("operator branch mismatch: row=%s local=%s", task.TargetBranch, branch)}
		}
		clean, err := r.git.IsClean(ctx, repoPath)
		if err != nil {
			return decision{State: StateFailed, Reason: "cannot read worktree status: " + err.Error()}
		}
		if !clean {
			return decision{State: StateFailed, Reason: "operator worktree is dirty"}
		}
	}
	return decision{Runnable: true, State: StateDiscovered, RepoPath: repoPath}
}

func (r *Runner) runTask(ctx context.Context, task Task, repoPath string, now time.Time) error {
	sessionID := r.cfg.RunnerID + "/" + NewEventID()
	eventID := NewEventID()
	r.log.Event(ctx, Event{EventID: eventID, Type: "discovered", State: StateDiscovered, TaskID: task.ID, SessionID: sessionID})
	r.log.Event(ctx, Event{Type: "claiming", State: StateClaiming, TaskID: task.ID, SessionID: sessionID})
	claim := Claim{
		TaskID:        task.ID,
		Version:       task.Version,
		SessionID:     sessionID,
		RunnerID:      r.cfg.RunnerID,
		LeaseUntil:    now.Add(r.cfg.LeaseDuration),
		CheckpointAt:  now,
		EventID:       eventID,
		ResumeCapsule: resumeCapsule(task, sessionID),
		DryRun:        r.cfg.DryRun,
	}
	result, err := r.notion.Claim(ctx, claim)
	if err != nil {
		return err
	}
	if !result.Claimed {
		r.log.Event(ctx, Event{Type: "claim_lost", State: StateReleased, TaskID: task.ID, SessionID: sessionID})
		return nil
	}
	r.log.Event(ctx, Event{Type: "claimed", State: StateClaimed, TaskID: task.ID, SessionID: sessionID, Fields: map[string]any{"dry_run": r.cfg.DryRun}})
	r.log.Event(ctx, Event{Type: "starting", State: StateStarting, TaskID: task.ID, SessionID: sessionID, Fields: map[string]any{"repo_path": repoPath, "role": task.Role}})
	r.log.Event(ctx, Event{Type: "running", State: StateRunning, TaskID: task.ID, SessionID: sessionID})
	launch, err := r.launcher.Launch(ctx, task, repoPath, sessionID)
	if err != nil {
		r.log.Event(ctx, Event{Type: "launch_error", State: StateFailed, TaskID: task.ID, SessionID: sessionID, Message: err.Error()})
		return nil
	}
	r.log.Event(ctx, Event{Type: "launcher_exit", State: StateRunning, TaskID: task.ID, SessionID: sessionID, Fields: map[string]any{"exit_code": launch.ExitCode}})

	refreshed, err := r.notion.Refresh(ctx, task.ID)
	if err != nil {
		return err
	}
	switch {
	case isCompleted(refreshed):
		r.log.Event(ctx, Event{Type: "verified_completed", State: StateCompleted, TaskID: task.ID, SessionID: sessionID})
	case isBlocked(refreshed):
		r.log.Event(ctx, Event{Type: "verified_blocked", State: StateBlocked, TaskID: task.ID, SessionID: sessionID})
	default:
		r.log.Event(ctx, Event{Type: "no_completion_evidence", State: StateFailed, TaskID: task.ID, SessionID: sessionID})
	}
	return nil
}

func isRunnableStatus(status string) bool {
	switch status {
	case "실행재개", "ready", "Ready":
		return true
	default:
		return false
	}
}

func deniedCategory(category string) bool {
	c := strings.ToLower(category)
	denied := []string{"ui", "ux", "deploy", "deployment", "delete", "deletion", "main push", "merge", "force push"}
	for _, item := range denied {
		if strings.Contains(c, item) {
			return true
		}
	}
	return false
}

func isCompleted(task Task) bool {
	switch strings.ToLower(task.CompletionState) {
	case "completed", "완료", "완료보고":
		return true
	}
	switch strings.ToLower(task.Status) {
	case "completed", "완료", "완료보고":
		return true
	default:
		return false
	}
}

func isBlocked(task Task) bool {
	switch strings.ToLower(task.CompletionState) {
	case "blocked", "차단", "design_gap":
		return true
	}
	switch strings.ToLower(task.Status) {
	case "blocked", "차단", "design_gap":
		return true
	default:
		return false
	}
}

func resumeCapsule(task Task, sessionID string) string {
	return fmt.Sprintf("session: %s\nrepo: %s\nbranch: %s\npr: %s\nphase: claimed\nnext exact action: launch local Codex template %s\n", sessionID, task.Repository, task.TargetBranch, task.PR, task.Role)
}
