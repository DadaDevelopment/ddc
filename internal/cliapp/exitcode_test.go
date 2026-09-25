package cliapp

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	if ExitCode(nil) != ExitPass {
		t.Error("nil must pass")
	}
	if ExitCode(errors.New("x")) != ExitFail {
		t.Error("plain error must fail")
	}
	if ExitCode(fmt.Errorf("wrapped: %w", ConfigErrorf("bad"))) != ExitConfig {
		t.Error("wrapped config error must keep exit 2")
	}
	for in, want := range map[int]int{0: 0, 1: 1, 2: 2, 3: 1, 137: 1, -1: 1} {
		if got := childExitCode(in); got != want {
			t.Errorf("childExitCode(%d) = %d, want %d", in, got, want)
		}
	}
}
