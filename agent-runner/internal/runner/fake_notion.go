package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

type FakeNotionAdapter struct {
	mu    sync.Mutex
	tasks map[string]Task
}

type fakeFile struct {
	Tasks []Task `json:"tasks"`
}

func NewFakeNotionAdapterFromFile(path string) (*FakeNotionAdapter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file fakeFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	return NewFakeNotionAdapter(file.Tasks), nil
}

func NewFakeNotionAdapter(tasks []Task) *FakeNotionAdapter {
	m := make(map[string]Task, len(tasks))
	for _, task := range tasks {
		m[task.ID] = task
	}
	return &FakeNotionAdapter{tasks: m}
}

func (a *FakeNotionAdapter) ListCandidates(_ context.Context, lane string) ([]Task, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []Task
	for _, task := range a.tasks {
		if task.Lane == lane {
			out = append(out, task)
		}
	}
	return out, nil
}

func (a *FakeNotionAdapter) Claim(_ context.Context, claim Claim) (ClaimResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	task, ok := a.tasks[claim.TaskID]
	if !ok {
		return ClaimResult{}, fmt.Errorf("task %q not found", claim.TaskID)
	}
	if task.Version != claim.Version {
		return ClaimResult{Claimed: false, Task: task}, nil
	}
	if task.SessionID != "" {
		return ClaimResult{Claimed: false, Task: task}, nil
	}
	if claim.DryRun {
		return ClaimResult{Claimed: true, Task: task}, nil
	}
	task.SessionID = claim.SessionID
	task.RunnerID = claim.RunnerID
	task.LeaseUntil = &claim.LeaseUntil
	task.CheckpointAt = &claim.CheckpointAt
	task.EventID = claim.EventID
	task.ResumeCapsule = claim.ResumeCapsule
	task.Version++
	a.tasks[task.ID] = task
	return ClaimResult{Claimed: true, Task: task}, nil
}

// SetTerminalEvidence models a fake Notion writer recording terminal evidence.
// Evidence must identify the claim session and its checkpoint event.
func (a *FakeNotionAdapter) SetTerminalEvidence(taskID, state, sessionID, eventID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	task, ok := a.tasks[taskID]
	if !ok {
		return errors.New("task not found")
	}
	task.CompletionState = state
	task.EvidenceSessionID = sessionID
	task.EvidenceEventID = eventID
	a.tasks[taskID] = task
	return nil
}

func (a *FakeNotionAdapter) Refresh(_ context.Context, taskID string) (Task, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	task, ok := a.tasks[taskID]
	if !ok {
		return Task{}, errors.New("task not found")
	}
	return task, nil
}
