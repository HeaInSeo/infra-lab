package runner

import (
	"context"
	"errors"
	"time"
)

const (
	LaneOperator  = "operator"
	LaneGuardrail = "guardrail"
)

const (
	StateIdle            = "IDLE"
	StateDiscovered      = "DISCOVERED"
	StateClaiming        = "CLAIMING"
	StateClaimed         = "CLAIMED"
	StateStarting        = "STARTING"
	StateRunning         = "RUNNING"
	StateCheckpointStale = "CHECKPOINT_STALE"
	StateCompleted       = "COMPLETED"
	StateBlocked         = "BLOCKED"
	StateRecovering      = "RECOVERING"
	StateReleased        = "RELEASED"
	StateFailed          = "FAILED"
)

type Config struct {
	Lane                string
	DryRun              bool
	Once                bool
	PollInterval        time.Duration
	LeaseDuration       time.Duration
	CheckpointFreshness time.Duration
	FakeNotionPath      string
	RunnerID            string
	RepoAllowlist       map[string]string
	Now                 func() time.Time
}

func (c Config) Validate() error {
	if c.Lane != LaneOperator && c.Lane != LaneGuardrail {
		return errors.New("lane must be operator or guardrail")
	}
	if c.PollInterval <= 0 {
		return errors.New("poll interval must be positive")
	}
	if c.LeaseDuration <= 0 {
		return errors.New("lease duration must be positive")
	}
	if c.CheckpointFreshness <= 0 {
		return errors.New("checkpoint freshness must be positive")
	}
	if c.FakeNotionPath == "" {
		return errors.New("phase 1 requires --fake-notion")
	}
	if len(c.RepoAllowlist) == 0 {
		return errors.New("at least one --repo allowlist entry is required")
	}
	return nil
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

type Task struct {
	ID                string     `json:"id"`
	Lane              string     `json:"lane"`
	Status            string     `json:"status"`
	Repository        string     `json:"repository"`
	DefaultHeadSHA    string     `json:"default_head_sha"`
	TargetBranch      string     `json:"target_branch"`
	PR                string     `json:"pr"`
	Role              string     `json:"role"`
	WorkCategory      string     `json:"work_category"`
	SessionID         string     `json:"session_id,omitempty"`
	LeaseUntil        *time.Time `json:"lease_until,omitempty"`
	CheckpointAt      *time.Time `json:"checkpoint_at,omitempty"`
	RunnerID          string     `json:"runner_id,omitempty"`
	EventID           string     `json:"event_id,omitempty"`
	ResumeCapsule     string     `json:"resume_capsule,omitempty"`
	Version           int64      `json:"version"`
	CompletionState   string     `json:"completion_state,omitempty"`
	EvidenceSessionID string     `json:"evidence_session_id,omitempty"`
	EvidenceEventID   string     `json:"evidence_event_id,omitempty"`
}

type Claim struct {
	TaskID        string
	Version       int64
	SessionID     string
	RunnerID      string
	LeaseUntil    time.Time
	CheckpointAt  time.Time
	EventID       string
	ResumeCapsule string
	DryRun        bool
}

type ClaimResult struct {
	Claimed bool
	Task    Task
}

type NotionAdapter interface {
	ListCandidates(ctx context.Context, lane string) ([]Task, error)
	Claim(ctx context.Context, claim Claim) (ClaimResult, error)
	Refresh(ctx context.Context, taskID string) (Task, error)
}

type Launcher interface {
	Launch(ctx context.Context, task Task, repoPath string, sessionID string) (LaunchResult, error)
}

type LaunchResult struct {
	ExitCode int
}
