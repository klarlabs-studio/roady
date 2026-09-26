package main

import (
	"os"
	"testing"
)

func runMain(t *testing.T, args ...string) (code int) {
	t.Helper()
	oldArgs, oldExit := os.Args, exit
	defer func() { os.Args, exit = oldArgs, oldExit }()
	code = -1
	exit = func(c int) { code = c }
	os.Args = append([]string{"roady"}, args...)
	main()
	return code
}

func TestMainSucceeds(t *testing.T) {
	if code := runMain(t, "--help"); code != -1 {
		t.Errorf("--help exited with %d", code)
	}
}

func TestMainExitsNonZeroOnError(t *testing.T) {
	if code := runMain(t, "invalid-cmd-999"); code != 1 {
		t.Errorf("an unknown command exited with %d, want 1", code)
	}
}
