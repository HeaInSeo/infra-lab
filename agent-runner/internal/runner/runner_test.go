package runner

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigRejectsInvalidLane(t *testing.T) {
	cfg := testConfig(t, nil)
	cfg.Lane = "ui"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid lane error")
	}
}

func TestDryRunDoesNotMutateFakeClaim(t *testing.T) {
	repo := newGitRepo(t, "agent-runner-dry-run")
	head := gitOutput(t, repo, "rev-parse", "origin/main")
	task := runnableTask(head)
	adapter := NewFakeNotionAdapter([]Task{task})
	var logs bytes.Buffer
	cfg := testConfig(t, map[string]string{task.Repository: repo})
	cfg.DryRun = true

	r := New(cfg, adapter, GitInspector{}, DryRunLauncher{}, NewJSONLogger(&logs))
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := adapter.Refresh(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.SessionID != "" {
		t.Fatalf("dry-run mutated session id: %q", got.SessionID)
	}
	if !strings.Contains(logs.String(), `"state":"CLAIMED"`) {
		t.Fatalf("expected claimed dry-run event, got logs:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), `"state":"FAILED"`) {
		t.Fatalf("Codex exit without Notion completion must not complete, logs:\n%s", logs.String())
	}
}

func TestSkipsTaskWithValidLease(t *testing.T) {
	repo := newGitRepo(t, "agent-runner-dry-run")
	head := gitOutput(t, repo, "rev-parse", "origin/main")
	lease := fixedNow().Add(time.Hour)
	task := runnableTask(head)
	task.SessionID = "other"
	task.LeaseUntil = &lease
	adapter := NewFakeNotionAdapter([]Task{task})
	var logs bytes.Buffer
	cfg := testConfig(t, map[string]string{task.Repository: repo})

	r := New(cfg, adapter, GitInspector{}, DryRunLauncher{}, NewJSONLogger(&logs))
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(logs.String(), `"state":"CLAIMED"`) {
		t.Fatalf("task with valid lease was claimed:\n%s", logs.String())
	}
}

func TestFreshCheckpointPreventsExpiredLeaseTakeover(t *testing.T) {
	repo := newGitRepo(t, "agent-runner-dry-run")
	head := gitOutput(t, repo, "rev-parse", "origin/main")
	lease := fixedNow().Add(-time.Minute)
	checkpoint := fixedNow().Add(-time.Minute)
	task := runnableTask(head)
	task.SessionID = "other"
	task.LeaseUntil = &lease
	task.CheckpointAt = &checkpoint
	adapter := NewFakeNotionAdapter([]Task{task})
	var logs bytes.Buffer
	cfg := testConfig(t, map[string]string{task.Repository: repo})

	r := New(cfg, adapter, GitInspector{}, DryRunLauncher{}, NewJSONLogger(&logs))
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(logs.String(), `"state":"CHECKPOINT_STALE"`) {
		t.Fatalf("expected checkpoint stale skip, got:\n%s", logs.String())
	}
}

func TestSkipsDeniedCategoryAndMissingRequiredFields(t *testing.T) {
	repo := newGitRepo(t, "agent-runner-dry-run")
	head := gitOutput(t, repo, "rev-parse", "origin/main")
	denied := runnableTask(head)
	denied.ID = "denied"
	denied.WorkCategory = "UI/UX"
	missing := runnableTask(head)
	missing.ID = "missing"
	missing.PR = ""
	adapter := NewFakeNotionAdapter([]Task{denied, missing})
	var logs bytes.Buffer
	cfg := testConfig(t, map[string]string{denied.Repository: repo})

	r := New(cfg, adapter, GitInspector{}, DryRunLauncher{}, NewJSONLogger(&logs))
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(logs.String(), `"state":"CLAIMED"`) {
		t.Fatalf("unsafe tasks were claimed:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "work category is denied") {
		t.Fatalf("expected denied category log, got:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "missing repository/default_head_sha/target_branch/pr") {
		t.Fatalf("expected missing field log, got:\n%s", logs.String())
	}
}

func TestRejectsMissingClaimContractFields(t *testing.T) {
	for _, field := range []string{"task_id", "role", "work_category"} {
		t.Run(field, func(t *testing.T) {
			task := runnableTask("unused")
			switch field {
			case "task_id":
				task.ID = ""
			case "role":
				task.Role = ""
			case "work_category":
				task.WorkCategory = ""
			}
			r := New(testConfig(t, nil), NewFakeNotionAdapter(nil), GitInspector{}, DryRunLauncher{}, NewJSONLogger(&bytes.Buffer{}))
			got := r.evaluate(context.Background(), task, fixedNow())
			if got.Runnable || got.State != StateFailed || got.Reason != "missing task_id/role/work_category" {
				t.Fatalf("malformed task decision = %#v", got)
			}
		})
	}
}

func TestDefaultHeadMismatchPreventsLaunch(t *testing.T) {
	repo := newGitRepo(t, "agent-runner-dry-run")
	task := runnableTask("0000000000000000000000000000000000000000")
	adapter := NewFakeNotionAdapter([]Task{task})
	var logs bytes.Buffer
	cfg := testConfig(t, map[string]string{task.Repository: repo})

	r := New(cfg, adapter, GitInspector{}, DryRunLauncher{}, NewJSONLogger(&logs))
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(logs.String(), `"state":"CLAIMED"`) {
		t.Fatalf("default head mismatch was claimed:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "default head mismatch") {
		t.Fatalf("expected default head mismatch log, got:\n%s", logs.String())
	}
}

func TestNonDryRunFakeClaimPersistsAtomicRecordAndCurrentEvidenceCompletes(t *testing.T) {
	repo := newGitRepo(t, "agent-runner-dry-run")
	head := gitOutput(t, repo, "rev-parse", "origin/main")
	task := runnableTask(head)
	adapter := NewFakeNotionAdapter([]Task{task})
	var logs bytes.Buffer
	cfg := testConfig(t, map[string]string{task.Repository: repo})
	cfg.DryRun = false

	r := New(cfg, adapter, GitInspector{}, evidenceLauncher{adapter: adapter, state: "completed"}, NewJSONLogger(&logs))
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := adapter.Refresh(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.SessionID == "" {
		t.Fatal("expected fake claim to record session id")
	}
	if got.RunnerID != cfg.RunnerID || got.EventID == "" || got.ResumeCapsule == "" || got.CheckpointAt == nil {
		t.Fatalf("incomplete atomic claim record: %#v", got)
	}
	if !strings.Contains(logs.String(), `"state":"COMPLETED"`) {
		t.Fatalf("expected completed evidence, got logs:\n%s", logs.String())
	}
}

func TestStaleTerminalEvidenceCannotCompleteNewSession(t *testing.T) {
	repo := newGitRepo(t, "agent-runner-dry-run")
	head := gitOutput(t, repo, "rev-parse", "refs/remotes/origin/HEAD")
	task := runnableTask(head)
	task.CompletionState = "completed"
	task.EvidenceSessionID = "previous-session"
	task.EvidenceEventID = "previous-event"
	adapter := NewFakeNotionAdapter([]Task{task})
	var logs bytes.Buffer
	cfg := testConfig(t, map[string]string{task.Repository: repo})
	cfg.DryRun = false

	r := New(cfg, adapter, GitInspector{}, DryRunLauncher{}, NewJSONLogger(&logs))
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(logs.String(), `"state":"COMPLETED"`) || !strings.Contains(logs.String(), `"state":"FAILED"`) {
		t.Fatalf("stale evidence was accepted:\n%s", logs.String())
	}
}

func TestDefaultHeadUsesConfiguredRemoteDefaultBranch(t *testing.T) {
	repo := newGitRepoWithRemote(t, "feature", "upstream", "trunk")
	want := gitOutput(t, repo, "rev-parse", "HEAD")
	got, err := (GitInspector{}).DefaultHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("DefaultHead: %v", err)
	}
	if got != want {
		t.Fatalf("DefaultHead = %q, want %q", got, want)
	}
}

type evidenceLauncher struct {
	adapter *FakeNotionAdapter
	state   string
}

func (l evidenceLauncher) Launch(ctx context.Context, task Task, _ string, sessionID string) (LaunchResult, error) {
	claimed, err := l.adapter.Refresh(ctx, task.ID)
	if err != nil {
		return LaunchResult{}, err
	}
	if err := l.adapter.SetTerminalEvidence(task.ID, l.state, sessionID, claimed.EventID); err != nil {
		return LaunchResult{}, err
	}
	return LaunchResult{}, nil
}

func testConfig(t *testing.T, repos map[string]string) Config {
	t.Helper()
	if repos == nil {
		repos = map[string]string{"HeaInSeo/infra-lab": t.TempDir()}
	}
	return Config{
		Lane:                LaneOperator,
		DryRun:              true,
		Once:                true,
		PollInterval:        time.Minute,
		LeaseDuration:       30 * time.Minute,
		CheckpointFreshness: 15 * time.Minute,
		FakeNotionPath:      "fake.json",
		RunnerID:            "test-runner",
		RepoAllowlist:       repos,
		Now:                 fixedNow,
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
}

func runnableTask(head string) Task {
	return Task{
		ID:             "task-1",
		Lane:           LaneOperator,
		Status:         "실행재개",
		Repository:     "HeaInSeo/infra-lab",
		DefaultHeadSHA: head,
		TargetBranch:   "agent-runner-dry-run",
		PR:             "https://github.com/HeaInSeo/infra-lab/pull/1",
		Role:           "operator",
		WorkCategory:   "backend",
		Version:        1,
	}
}

func newGitRepo(t *testing.T, branch string) string {
	return newGitRepoWithRemote(t, branch, "origin", "main")
}

func newGitRepoWithRemote(t *testing.T, branch, remote, defaultBranch string) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", branch)
	runGit(t, dir, "config", "user.email", "agent-runner@example.invalid")
	runGit(t, dir, "config", "user.name", "agent-runner test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")
	head := gitOutput(t, dir, "rev-parse", "HEAD")
	if err := os.MkdirAll(filepath.Join(dir, ".git", "refs", "remotes", remote), 0o755); err != nil {
		t.Fatalf("mkdir refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "refs", "remotes", remote, defaultBranch), []byte(head+"\n"), 0o644); err != nil {
		t.Fatalf("write remote default ref: %v", err)
	}
	runGit(t, dir, "remote", "add", remote, filepath.Join(t.TempDir(), "unused.git"))
	runGit(t, dir, "config", "branch."+branch+".remote", remote)
	runGit(t, dir, "symbolic-ref", "refs/remotes/"+remote+"/HEAD", "refs/remotes/"+remote+"/"+defaultBranch)
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}
