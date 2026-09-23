package api

import (
	"strconv"
	"strings"
	"time"
)

// RetryPolicy is how HTTP calls back off on 429 and transient failures.
type RetryPolicy struct {
	Max429       uint
	MaxTransient uint
	BaseDelay    time.Duration
	Sleep        func(time.Duration)
}

// DefaultRetry is the production policy (TS throttle + flow timeouts).
func DefaultRetry() RetryPolicy {
	return RetryPolicy{
		Max429:       5,
		MaxTransient: 3,
		BaseDelay:    200 * time.Millisecond,
		Sleep:        time.Sleep,
	}
}

// TestRetry does not sleep.
func TestRetry() RetryPolicy {
	return RetryPolicy{
		Max429:       5,
		MaxTransient: 3,
		BaseDelay:    time.Millisecond,
		Sleep:        func(time.Duration) {},
	}
}

func (p RetryPolicy) sleep(d time.Duration) {
	if p.Sleep != nil {
		p.Sleep(d)
	}
}

func (p RetryPolicy) delayFor(attempt uint, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > 60*time.Second {
			return 60 * time.Second
		}
		return retryAfter
	}
	shift := attempt
	if shift > 4 {
		shift = 4
	}
	return p.BaseDelay * time.Duration(1<<shift)
}

func parseRetryAfter(header string) time.Duration {
	raw := strings.TrimSpace(header)
	if raw == "" {
		return 0
	}
	secs, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func isTransientStatus(status int) bool {
	return status >= 502 && status <= 504
}
