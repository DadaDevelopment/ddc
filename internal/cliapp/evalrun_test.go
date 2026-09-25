package cliapp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const fakeRunner = `#!/bin/sh
suite=""
prev=""
for a in "$@"; do
  [ "$prev" = "--suite" ] && suite="$a"
  prev="$a"
done
echo "$*" >> "$FAKE_LOG"
eval "code=\${FAKE_EXIT_$suite:-0}"
exit "$code"
`

func evalFixture(t *testing.T, suites ...string) (AgentOptions, string) {
	t.Helper()
	repo := t.TempDir()
	dir := filepath.Join(repo, "agents", "sample")
	for _, d := range []string{filepath.Join(dir, "evals", "suites"), filepath.Join(repo, "scripts")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string, mode os.FileMode) {
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "core.md"), "be helpful", 0o644)
	write(filepath.Join(repo, evalEntrypoint), "", 0o644)
	for _, s := range suites {
		write(filepath.Join(dir, "evals", "suites", s+".yaml"), "cases: []", 0o644)
	}
	python := filepath.Join(t.TempDir(), "fake-python")
	write(python, fakeRunner, 0o755)
	logPath := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("DDC_PYTHON", python)
	t.Setenv("FAKE_LOG", logPath)
	t.Setenv("DDC_EVAL_ARGS", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	return AgentOptions{Repo: repo, Port: port}, logPath
}

func calls(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func TestAgentEvalRunsEverySuite(t *testing.T) {
	opts, logPath := evalFixture(t, "markers", "script")
	if err := AgentEval(context.Background(), Config{}, opts, io.Discard); err != nil {
		t.Fatalf("AgentEval: %v", err)
	}
	got := calls(t, logPath)
	if len(got) != 2 {
		t.Fatalf("runner calls = %d, want 2: %q", len(got), got)
	}
	for i, suite := range []string{"markers", "script"} {
		if !strings.Contains(got[i], "--suite "+suite) {
			t.Errorf("call %d = %q, want --suite %s", i, got[i], suite)
		}
		if !strings.Contains(got[i], "--output-dir "+filepath.Join(defaultEvalOutputDir, suite)) {
			t.Errorf("call %d = %q, want a per-suite output dir", i, got[i])
		}
	}
}

func TestAgentEvalAggregatesExitCodes(t *testing.T) {
	cases := []struct {
		name  string
		exits map[string]string
		want  int
	}{
		{"all pass", nil, ExitPass},
		{"one fails", map[string]string{"markers": "1"}, ExitFail},
		{"crash is a failure", map[string]string{"script": "137"}, ExitFail},
		{"config error wins", map[string]string{"markers": "1", "script": "2"}, ExitConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, logPath := evalFixture(t, "markers", "script")
			for suite, code := range tc.exits {
				t.Setenv("FAKE_EXIT_"+suite, code)
			}
			err := AgentEval(context.Background(), Config{}, opts, io.Discard)
			if got := ExitCode(err); got != tc.want {
				t.Fatalf("ExitCode = %d (%v), want %d", got, err, tc.want)
			}
			if n := len(calls(t, logPath)); n != 2 {
				t.Fatalf("runner calls = %d, want every suite to run", n)
			}
		})
	}
}

func TestAgentEvalSingleSuite(t *testing.T) {
	opts, logPath := evalFixture(t, "markers", "script")
	opts.Suite = "script"
	t.Setenv("DDC_EVAL_ARGS", "--output-dir out")
	t.Setenv("FAKE_EXIT_script", "2")
	err := AgentEval(context.Background(), Config{}, opts, io.Discard)
	if ExitCode(err) != ExitConfig {
		t.Fatalf("ExitCode = %d (%v), want runner exit 2 propagated", ExitCode(err), err)
	}
	got := calls(t, logPath)
	if len(got) != 1 || !strings.Contains(got[0], "--suite script --output-dir out") {
		t.Fatalf("calls = %q", got)
	}
}

func TestAgentEvalConfigErrors(t *testing.T) {
	opts, _ := evalFixture(t, "markers")
	opts.Suite = "nope"
	if code := ExitCode(AgentEval(context.Background(), Config{}, opts, io.Discard)); code != ExitConfig {
		t.Errorf("unknown suite: exit %d, want 2", code)
	}
	opts.Suite = ""
	opts.Port = 1
	if code := ExitCode(AgentEval(context.Background(), Config{}, opts, io.Discard)); code != ExitConfig {
		t.Errorf("no local agent: exit %d, want 2", code)
	}
}

func TestSuiteOutputArgs(t *testing.T) {
	cases := []struct {
		extra []string
		want  []string
	}{
		{nil, []string{"--output-dir", filepath.Join("eval-artifacts", "s")}},
		{[]string{"--label", "ci", "--output-dir", "r"}, []string{"--label", "ci", "--output-dir", filepath.Join("r", "s")}},
		{[]string{"--output-dir=r", "--repeats", "3"}, []string{"--repeats", "3", "--output-dir", filepath.Join("r", "s")}},
	}
	for _, tc := range cases {
		if got := suiteOutputArgs(tc.extra, "s"); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("suiteOutputArgs(%q) = %q, want %q", tc.extra, got, tc.want)
		}
	}
}
