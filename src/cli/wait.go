package cli

import (
	"flag"
	"io"
	"os"
	"sync"
	"time"

	"github.com/polyapi/polyglot/src/api"
	"github.com/spf13/cobra"
)

var waitFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const (
	waitShowAfter = 150 * time.Millisecond
	waitTickEvery = 80 * time.Millisecond
)

// WaitSpinner is a deferred line spinner for in-flight HTTP.
// Fast calls never paint; a shown spinner is cleared before the wait callback returns
// so the command's result can replace that line.
type WaitSpinner struct {
	w         io.Writer
	showAfter time.Duration
	tickEvery time.Duration

	mu        sync.Mutex
	n         int
	shown     bool
	frame     int
	showTimer *time.Timer
	ticker    *time.Ticker
	quit      chan struct{}
}

// NewWaitSpinner writes frames to w. Exported for tests.
func NewWaitSpinner(w io.Writer) *WaitSpinner {
	return NewWaitSpinnerTiming(w, waitShowAfter, waitTickEvery)
}

// NewWaitSpinnerTiming is NewWaitSpinner with explicit delays. Exported for tests.
func NewWaitSpinnerTiming(w io.Writer, showAfter, tickEvery time.Duration) *WaitSpinner {
	if showAfter <= 0 {
		showAfter = time.Millisecond
	}
	if tickEvery <= 0 {
		tickEvery = 80 * time.Millisecond
	}
	return &WaitSpinner{w: w, showAfter: showAfter, tickEvery: tickEvery}
}

// Set matches api.SetRequestWait: true begins a wait, false ends one.
func (s *WaitSpinner) Set(start bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if start {
		s.n++
		if s.n == 1 && !s.shown && s.showTimer == nil {
			s.showTimer = time.AfterFunc(s.showAfter, s.start)
		}
		return
	}
	if s.n > 0 {
		s.n--
	}
	if s.n > 0 {
		return
	}
	if s.showTimer != nil {
		s.showTimer.Stop()
		s.showTimer = nil
	}
	s.stopLocked()
}

func (s *WaitSpinner) start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.n <= 0 || s.shown {
		return
	}
	s.shown = true
	s.showTimer = nil
	s.frame = 0
	s.quit = make(chan struct{})
	s.ticker = time.NewTicker(s.tickEvery)
	go s.loop(s.quit, s.ticker)
	s.renderLocked()
}

func (s *WaitSpinner) stopLocked() {
	if s.quit != nil {
		close(s.quit)
		s.quit = nil
	}
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	if s.shown {
		s.clearLocked()
		s.shown = false
	}
}

func (s *WaitSpinner) loop(quit <-chan struct{}, ticker *time.Ticker) {
	for {
		select {
		case <-quit:
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.shown {
				s.frame++
				s.renderLocked()
			}
			s.mu.Unlock()
		}
	}
}

func (s *WaitSpinner) renderLocked() {
	frame := waitFrames[s.frame%len(waitFrames)]
	line := "\r" + paint(River600, false, frame) + " " + infoText("waiting")
	_, _ = io.WriteString(s.w, line)
}

func (s *WaitSpinner) clearLocked() {
	_, _ = io.WriteString(s.w, "\r\033[2K")
}

func enableRequestWait(cmd *cobra.Command) {
	if !shouldShowRequestWait(cmd) {
		return
	}
	w := requestWaitWriter(cmd)
	if w == nil {
		return
	}
	api.SetRequestWait(NewWaitSpinner(w).Set)
}

func shouldShowRequestWait(cmd *cobra.Command) bool {
	if globalsFrom(cmd).Quiet {
		return false
	}
	if os.Getenv("CI") != "" {
		return false
	}
	if flag.Lookup("test.v") != nil {
		return false
	}
	return requestWaitWriter(cmd) != nil
}

func requestWaitWriter(cmd *cobra.Command) io.Writer {
	if cmd != nil {
		if writerIsTTY(cmd.ErrOrStderr()) {
			return cmd.ErrOrStderr()
		}
		if writerIsTTY(cmd.OutOrStdout()) {
			return cmd.OutOrStdout()
		}
	}
	if isCharDevice(os.Stderr) {
		return os.Stderr
	}
	return nil
}

func disableRequestWait() {
	api.SetRequestWait(nil)
}

// WaitShown is true when the spinner has been painted. Exported for tests.
func (s *WaitSpinner) WaitShown() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shown
}
