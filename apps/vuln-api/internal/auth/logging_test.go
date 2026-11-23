package auth

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// captureLogs routes the default slog logger into a buffer for one test and
// returns a function that parses every JSON line written so far.
func captureLogs(t *testing.T) func() []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return func() []map[string]any {
		var out []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("log line is not JSON: %q", line)
			}
			out = append(out, m)
		}
		return out
	}
}

func findEvent(logs []map[string]any, event string) map[string]any {
	for _, l := range logs {
		if l["event"] == event {
			return l
		}
	}
	return nil
}

func TestPasswordLoginLogsFailure(t *testing.T) {
	logs := captureLogs(t)
	o, _, _, _ := setup(t)
	passwordLogin(o, "192.0.2.10:1", `{"username":"Ana","password":"wrong"}`)
	e := findEvent(logs(), "auth.login_failed")
	if e == nil {
		t.Fatalf("no auth.login_failed event in %v", logs())
	}
	if e["username"] != "ana" || e["ip"] != "192.0.2.10" || e["level"] != "WARN" {
		t.Fatalf("event fields: %v", e)
	}
	for k, v := range e {
		if s, ok := v.(string); ok && strings.Contains(s, "wrong") {
			t.Fatalf("password leaked into log field %q", k)
		}
	}
}

func TestPasswordLoginLogsRateLimit(t *testing.T) {
	o, _, _, _ := setup(t)
	for i := 0; i < 10; i++ {
		passwordLogin(o, "192.0.2.11:1", `{"username":"ana","password":"guess"}`)
	}
	logs := captureLogs(t)
	passwordLogin(o, "192.0.2.12:1", `{"username":"ana","password":"guess"}`)
	e := findEvent(logs(), "auth.login_rate_limited")
	if e == nil || e["username"] != "ana" || e["ip"] != "192.0.2.12" {
		t.Fatalf("rate-limit event: %v", logs())
	}
}

func TestPasswordLoginLogsSuccess(t *testing.T) {
	logs := captureLogs(t)
	o, _, _, _ := setup(t)
	passwordLogin(o, "192.0.2.13:1", `{"username":"ana","password":"correct horse"}`)
	e := findEvent(logs(), "auth.login_succeeded")
	if e == nil || e["sub"] != "user-123" || e["ip"] != "192.0.2.13" || e["method"] != "password" {
		t.Fatalf("success event: %v", logs())
	}
}
