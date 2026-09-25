package main

import (
	"context"
	"io"
	"testing"

	"github.com/dada-tuda/ddc/internal/cliapp"
)

func TestRunAgentUsageErrorsExit2(t *testing.T) {
	cases := [][]string{
		{"frobnicate"},
		{"spec", "--nope"},
		{"eval", "--port", "abc"},
		{"invoke", "--text"},
	}
	for _, args := range cases {
		err := runAgent(context.Background(), cliapp.Config{}, args, io.Discard)
		if got := cliapp.ExitCode(err); got != cliapp.ExitConfig {
			t.Errorf("runAgent(%q) exit %d (%v), want 2", args, got, err)
		}
	}
}
