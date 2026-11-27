package main

import (
	"flag"
	"io"
	"testing"
)

func TestParseFlagsHelpIsSuccess(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Bool("x", false, "")

	if stop, code := parseFlags(fs, []string{"--help"}); !stop || code != 0 {
		t.Fatalf("--help: stop=%v code=%d, want true 0", stop, code)
	}
	if stop, code := parseFlags(fs, []string{"--nope"}); !stop || code != 2 {
		t.Fatalf("bad flag: stop=%v code=%d, want true 2", stop, code)
	}
	if stop, _ := parseFlags(fs, []string{"--x"}); stop {
		t.Fatal("valid flag: should continue, not stop")
	}
}

func TestParseFlagsArgsRequiresPositional(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if stop, code := parseFlagsArgs(fs, nil, 1); !stop || code != 2 {
		t.Fatalf("missing arg: stop=%v code=%d, want true 2", stop, code)
	}
	if stop, _ := parseFlagsArgs(fs, []string{"id"}, 1); stop {
		t.Fatal("with arg: should continue")
	}
}
