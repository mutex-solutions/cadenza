package main

import (
	"strings"
	"testing"
)

func TestRunCommands(t *testing.T) {
	if err := run([]string{"version"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"help"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"nope"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unknown command: %v", err)
	}
	if err := run([]string{"owner"}); err == nil {
		t.Fatal("owner without a subcommand must fail")
	}
	t.Setenv("CADENZA_PUBLIC_URL", "not a url")
	if err := run([]string{"serve"}); err == nil {
		t.Fatal("bad config must fail")
	}
}
