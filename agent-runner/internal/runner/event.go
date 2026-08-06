package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
)

type Event struct {
	EventID   string         `json:"event_id"`
	Time      time.Time      `json:"time"`
	Type      string         `json:"type"`
	State     string         `json:"state"`
	TaskID    string         `json:"task_id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Message   string         `json:"message,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type JSONLogger struct {
	mu  sync.Mutex
	out io.Writer
}

func NewJSONLogger(out io.Writer) *JSONLogger {
	return &JSONLogger{out: out}
}

func (l *JSONLogger) Event(_ context.Context, e Event) {
	if e.EventID == "" {
		e.EventID = NewEventID()
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	e.Fields = redactMap(e.Fields)
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = json.NewEncoder(l.out).Encode(e)
}

func NewEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	return hex.EncodeToString(b[:])
}

func redactMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = v
	}
	return out
}
