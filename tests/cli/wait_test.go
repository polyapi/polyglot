package cli_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/polyapi/polyglot/src/cli"
)

func TestWaitSpinnerStaysOffWhenCancelledQuickly(t *testing.T) {
	var buf bytes.Buffer
	s := cli.NewWaitSpinnerTiming(&buf, 50*time.Millisecond, 20*time.Millisecond)
	s.Set(true)
	s.Set(false)
	time.Sleep(80 * time.Millisecond)
	if s.WaitShown() {
		t.Fatal("shown")
	}
	if buf.Len() != 0 {
		t.Fatalf("painted %q", buf.String())
	}
}

func TestWaitSpinnerPaintsThenClears(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	s := cli.NewWaitSpinnerTiming(&buf, 20*time.Millisecond, 20*time.Millisecond)
	s.Set(true)
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) && !s.WaitShown() {
		time.Sleep(10 * time.Millisecond)
	}
	if !s.WaitShown() {
		t.Fatal("expected spinner")
	}
	s.Set(false)
	got := buf.String()
	if !strings.Contains(got, "waiting") {
		t.Fatalf("missing waiting: %q", got)
	}
	if !strings.Contains(got, "\033[2K") {
		t.Fatalf("missing clear: %q", got)
	}
	if s.WaitShown() {
		t.Fatal("still shown after end")
	}
}

func TestWaitSpinnerRefcountKeepsOneLine(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	s := cli.NewWaitSpinnerTiming(&buf, 15*time.Millisecond, 20*time.Millisecond)
	s.Set(true)
	s.Set(true)
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) && !s.WaitShown() {
		time.Sleep(10 * time.Millisecond)
	}
	if !s.WaitShown() {
		t.Fatal("expected spinner")
	}
	s.Set(false)
	if !s.WaitShown() {
		t.Fatal("ended too early")
	}
	s.Set(false)
	if s.WaitShown() {
		t.Fatal("still shown")
	}
}
