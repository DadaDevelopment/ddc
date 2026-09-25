package cliapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func invokeServer(t *testing.T, status int, reply string) (Config, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/resolve":
			_, _ = w.Write([]byte(`{"project":{"id":"p1"},"environment":{"id":"e1"}}`))
		case r.URL.Path == "/api/v1/projects/p1/environments/e1/agents":
			_, _ = w.Write([]byte(`{"items":[{"name":"support"}]}`))
		case r.URL.Path == "/api/v1/agents/support/message":
			var body struct {
				Text string `json:"text"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			seen = append(seen, "text="+body.Text)
			w.WriteHeader(status)
			if status == http.StatusOK {
				_ = json.NewEncoder(w).Encode(map[string]string{"reply": reply})
			} else {
				_, _ = w.Write([]byte(`{"error":"agent did not answer"}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DDC_TOKEN", "tok")
	return Config{APIBase: srv.URL + "/api/v1"}, &seen
}

func TestAgentInvokePrintsReply(t *testing.T) {
	cfg, seen := invokeServer(t, http.StatusOK, "здравствуйте")
	var out bytes.Buffer
	opts := AgentOptions{Agent: "support", Text: "привет", Project: "demo", Env: "prod"}
	if err := AgentInvoke(context.Background(), cfg, opts, &out); err != nil {
		t.Fatalf("AgentInvoke: %v", err)
	}
	if strings.TrimSpace(out.String()) != "здравствуйте" {
		t.Fatalf("printed %q", out.String())
	}
	joined := strings.Join(*seen, "\n")
	for _, want := range []string{"POST /api/v1/agents/support/message Bearer tok", "text=привет", "GET /api/v1/resolve"} {
		if !strings.Contains(joined, want) {
			t.Errorf("requests %q lack %q", joined, want)
		}
	}
}

func TestAgentInvokeFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		reply  string
		opts   AgentOptions
		want   int
	}{
		{"agent error", http.StatusBadGateway, "", AgentOptions{Agent: "support", Text: "hi"}, ExitFail},
		{"forbidden", http.StatusForbidden, "", AgentOptions{Agent: "support", Text: "hi"}, ExitFail},
		{"empty reply", http.StatusOK, " ", AgentOptions{Agent: "support", Text: "hi"}, ExitFail},
		{"unknown agent in env", http.StatusOK, "x", AgentOptions{Agent: "other", Text: "hi", Project: "demo", Env: "prod"}, ExitFail},
		{"no text", http.StatusOK, "x", AgentOptions{Agent: "support"}, ExitConfig},
		{"project without env", http.StatusOK, "x", AgentOptions{Agent: "support", Text: "hi", Project: "demo"}, ExitConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := invokeServer(t, tc.status, tc.reply)
			err := AgentInvoke(context.Background(), cfg, tc.opts, &bytes.Buffer{})
			if got := ExitCode(err); got != tc.want {
				t.Fatalf("ExitCode = %d (%v), want %d", got, err, tc.want)
			}
		})
	}
}
