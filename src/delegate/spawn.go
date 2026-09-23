package delegate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Job is one adapter invocation.
type Job struct {
	Argv     []string
	Cwd      string
	Stdin    string
	Timeout  time.Duration
	ExtraEnv [][2]string
}

// RawOutput is captured adapter stdout/stderr and status.
type RawOutput struct {
	Status   *int
	Stdout   string
	Stderr   string
	TimedOut bool
}

// Runner runs an adapter job. Tests inject a fake.
type Runner interface {
	Run(job Job) (RawOutput, error)
}

// ProcessRunner spawns a real subprocess, stdin JSON in, stdout JSON out.
type ProcessRunner struct{}

func (ProcessRunner) Run(job Job) (RawOutput, error) {
	return runProcess(job)
}

func runProcess(job Job) (RawOutput, error) {
	if len(job.Argv) == 0 {
		return RawOutput{}, usageErr("empty adapter command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), job.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, job.Argv[0], job.Argv[1:]...)
	cmd.Dir = job.Cwd
	cmd.Stdin = strings.NewReader(job.Stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = osEnvironWith(map[string]string{
		"POLY_DELEGATE":          "1",
		"POLY_DELEGATE_PROTOCOL": strconv.FormatUint(uint64(Protocol), 10),
	}, job.ExtraEnv)
	setProcessGroup(cmd)

	err := cmd.Start()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || isNotFound(err) {
			return RawOutput{}, missingCommand(job.Argv, "")
		}
		return RawOutput{}, ioErr(err.Error())
	}
	waitErr := cmd.Wait()
	out := RawOutput{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		killProcess(cmd)
		out.TimedOut = true
		return out, nil
	}
	if waitErr != nil {
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			code := ee.ExitCode()
			out.Status = &code
			return out, nil
		}
		if errors.Is(waitErr, exec.ErrNotFound) || isNotFound(waitErr) {
			return RawOutput{}, missingCommand(job.Argv, "")
		}
		return RawOutput{}, ioErr(waitErr.Error())
	}
	zero := 0
	out.Status = &zero
	return out, nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "file not found")
}

func osEnvironWith(set map[string]string, extra [][2]string) []string {
	env := osEnviron()
	index := map[string]int{}
	for i, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		index[k] = i
	}
	put := func(k, v string) {
		if i, ok := index[k]; ok {
			env[i] = k + "=" + v
			return
		}
		index[k] = len(env)
		env = append(env, k+"="+v)
	}
	for k, v := range set {
		put(k, v)
	}
	for _, pair := range extra {
		put(pair[0], pair[1])
	}
	return env
}

func osEnviron() []string {
	return append([]string{}, os.Environ()...)
}

func looksLikeMissingModule(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "no module named") ||
		strings.Contains(s, "cannot find module") ||
		strings.Contains(s, "modulenotfounderror") ||
		strings.Contains(s, "err_module_not_found") ||
		(strings.Contains(s, "npx") && (strings.Contains(s, "could not determine") || strings.Contains(s, "not found"))) ||
		strings.Contains(s, "is not recognized as an internal or external command")
}

func missingAfterSpawn(argv []string, lang Language, output RawOutput) *Error {
	if output.TimedOut {
		return nil
	}
	failed := true
	if output.Status != nil && *output.Status == 0 {
		failed = false
	}
	if failed && looksLikeMissingModule(output.Stderr) {
		return missingCommand(argv, lang)
	}
	return nil
}

func timeoutFor(op Op, overrideSecs uint64) time.Duration {
	if overrideSecs > 0 {
		if overrideSecs < 1 {
			overrideSecs = 1
		}
		return time.Duration(overrideSecs) * time.Second
	}
	return defaultTimeout(op)
}

func defaultTimeout(op Op) time.Duration {
	switch op {
	case OpGenerate:
		return 300 * time.Second
	default:
		return 120 * time.Second
	}
}

func timeoutOverrideFromEnv() uint64 {
	s := os.Getenv("POLY_DELEGATE_TIMEOUT_SECS")
	if s == "" {
		return 0
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n == 0 {
		return 0
	}
	return n
}
