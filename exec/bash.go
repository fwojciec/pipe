package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/fwojciec/pipe"
)

const rollingBufSize = 2 * DefaultMaxBytes // 100KB rolling buffer

// bashExecutorArgs holds the arguments for bash command execution.
type bashExecutorArgs struct {
	Command  string `json:"command"`
	Timeout  int    `json:"timeout"`
	CheckPID int    `json:"check_pid"`
	KillPID  int    `json:"kill_pid"`
}

// BashExecutorTool returns the tool definition with background parameters.
func BashExecutorTool() pipe.Tool {
	return pipe.Tool{
		Name: "bash",
		Description: fmt.Sprintf(
			"Execute a bash command. Output truncated to last %d lines or %dKB; "+
				"if truncated, full output saved to temp file readable with the read tool. "+
				"Commands exceeding timeout are auto-backgrounded.",
			DefaultMaxLines, DefaultMaxBytes/1024,
		),
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"description": "The bash command to execute"
				},
				"timeout": {
					"type": "integer",
					"description": "Timeout in milliseconds before auto-backgrounding (default: 120000)"
				},
				"check_pid": {
					"type": "integer",
					"description": "Check on a backgrounded process and return new output"
				},
				"kill_pid": {
					"type": "integer",
					"description": "Kill a backgrounded process and return final output"
				}
			}
		}`),
	}
}

// BashExecutor executes bash commands with background process management.
type BashExecutor struct {
	bg *BackgroundRegistry
}

// NewBashExecutor creates a BashExecutor with a fresh background registry.
func NewBashExecutor() *BashExecutor {
	return &BashExecutor{bg: NewBackgroundRegistry()}
}

// Execute runs a bash command or manages a background process.
func (e *BashExecutor) Execute(ctx context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a bashExecutorArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	switch {
	case a.CheckPID > 0:
		return e.bg.Check(a.CheckPID)
	case a.KillPID > 0:
		return e.bg.Kill(a.KillPID)
	case a.Command != "":
		return e.runCommand(ctx, a)
	default:
		return domainError("one of command, check_pid, or kill_pid is required"), nil
	}
}

func (e *BashExecutor) runCommand(ctx context.Context, a bashExecutorArgs) (*pipe.ToolResult, error) {
	timeout := 120 * time.Second
	if a.Timeout > 0 {
		timeout = time.Duration(a.Timeout) * time.Millisecond
	}

	// Use exec.Command (not CommandContext) so timeout doesn't auto-kill —
	// we want to auto-background instead.
	cmd := osexec.Command("bash", "-c", a.Command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Create pipes manually instead of using cmd.StdoutPipe/StderrPipe so
	// that cmd.Wait() doesn't close the read ends before io.Copy finishes.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return domainError(fmt.Sprintf("failed to create stdout pipe: %s", err)), nil
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		stdoutR.Close()
		stdoutW.Close()
		return domainError(fmt.Sprintf("failed to create stderr pipe: %s", err)), nil
	}
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	if err := cmd.Start(); err != nil {
		stdoutR.Close()
		stdoutW.Close()
		stderrR.Close()
		stderrW.Close()
		return domainError(fmt.Sprintf("failed to start command: %s", err)), nil
	}

	// Close write ends in parent; child has its own copies.
	stdoutW.Close()
	stderrW.Close()

	stdoutC := NewOutputCollector(int64(DefaultMaxBytes), rollingBufSize)
	stderrC := NewOutputCollector(int64(DefaultMaxBytes), rollingBufSize)

	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	// io.Copy errors are intentionally ignored: the only read error is io.EOF
	// (pipe closed when process exits), and write errors are tracked by
	// OutputCollector.Err() which is checked when formatting results.
	go func() { _, _ = io.Copy(stdoutC, stdoutR); stdoutR.Close(); close(stdoutDone) }()
	go func() { _, _ = io.Copy(stderrC, stderrR); stderrR.Close(); close(stderrDone) }()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case waitErr := <-waitCh:
		// Shell exited. Give io.Copy goroutines a short grace period to drain
		// any remaining pipe buffer. If shell-backgrounded children (e.g.,
		// "sleep 5 & echo done") keep inherited FDs open, force-close the read
		// ends to unblock io.Copy rather than waiting for the children to exit.
		pipesDone := make(chan struct{})
		go func() { <-stdoutDone; <-stderrDone; close(pipesDone) }()
		select {
		case <-pipesDone:
			stdoutR.Close()
			stderrR.Close()
		case <-time.After(100 * time.Millisecond):
			stdoutR.Close()
			stderrR.Close()
			<-stdoutDone
			<-stderrDone
		}
		stdoutC.Close()
		stderrC.Close()
		return e.formatCompletedResult(waitErr, stdoutC, stderrC), nil

	case <-timer.C:
		// Timeout: auto-background.
		pid := cmd.Process.Pid
		bg := &BackgroundProcess{
			cmd:        cmd,
			stdout:     stdoutC,
			stderr:     stderrC,
			waitCh:     waitCh,
			stdoutDone: stdoutDone,
			stderrDone: stderrDone,
			doneCh:     make(chan struct{}),
		}
		go bg.watch()
		e.bg.Register(pid, bg)

		stdoutStr, _ := processOutput(stdoutC)
		stderrStr, _ := processOutput(stderrC)

		var b strings.Builder
		fmt.Fprintf(&b, "[Command backgrounded after %s timeout (pid %d).\n", timeout, pid)
		if stdoutStr != "" {
			fmt.Fprintf(&b, "\nstdout (partial):\n%s\n", stdoutStr)
		}
		if stderrStr != "" {
			fmt.Fprintf(&b, "\nstderr (partial):\n%s\n", stderrStr)
		}
		b.WriteString("\nUse check_pid or kill_pid to manage.]")

		return &pipe.ToolResult{
			Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
			IsError: false,
		}, nil

	case <-ctx.Done():
		// External cancellation: kill.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-waitCh
		<-stdoutDone
		<-stderrDone
		stdoutC.Close()
		stderrC.Close()
		return domainError(fmt.Sprintf("command cancelled: %s", ctx.Err())), nil
	}
}

func (e *BashExecutor) formatCompletedResult(waitErr error, stdout, stderr *OutputCollector) *pipe.ToolResult {
	exitCode := 0
	isError := false
	if waitErr != nil {
		isError = true
		var exitErr *osexec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return formatResult(exitCode, isError, stdout, stderr)
}

// processOutput sanitizes and truncates collector output. Returns the processed
// string and truncation metadata. For running processes, this returns a snapshot;
// the collector's Bytes() and TotalNewlines() calls are independently locked, so
// the line count may be slightly inconsistent with the content.
func processOutput(c *OutputCollector) (string, TruncateResult) {
	raw := string(c.Bytes())
	clean := Sanitize(raw)
	tr := TruncateTail(clean, DefaultMaxLines, DefaultMaxBytes)
	// Override total lines with the collector's accurate count (rolling buffer
	// may have dropped early data). TotalNewlines() counts \n characters; add 1
	// for an unterminated final line.
	total := c.TotalNewlines()
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		total++
	}
	tr.TotalLines = total
	return tr.Content, tr
}

func formatResult(exitCode int, isError bool, stdout, stderr *OutputCollector) *pipe.ToolResult {
	stdoutStr, stdoutTR := processOutput(stdout)
	stderrStr, stderrTR := processOutput(stderr)

	var b strings.Builder
	if stdoutStr != "" {
		fmt.Fprintf(&b, "stdout:\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(&b, "stderr:\n%s\n", stderrStr)
	}
	fmt.Fprintf(&b, "exit code: %d", exitCode)

	appendOffloadNotice(&b, "stdout", stdoutTR, stdout)
	appendOffloadNotice(&b, "stderr", stderrTR, stderr)

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
		IsError: isError,
	}
}

func appendOffloadNotice(b *strings.Builder, name string, tr TruncateResult, c *OutputCollector) {
	filePath := c.FilePath()
	offloadErr := c.Err()

	if !tr.Truncated && filePath == "" {
		return
	}
	if filePath != "" && offloadErr == nil {
		fmt.Fprintf(b, "\n[%s: Showing last %d of %d lines. Full output: %s]",
			name, tr.OutputLines, tr.TotalLines, filePath)
	} else if filePath != "" && offloadErr != nil {
		fmt.Fprintf(b, "\n[%s: Showing last %d of %d lines. Full output file may be incomplete: %s (%s)]",
			name, tr.OutputLines, tr.TotalLines, filePath, offloadErr)
	} else if tr.Truncated {
		fmt.Fprintf(b, "\n[%s: Showing last %d of %d lines]",
			name, tr.OutputLines, tr.TotalLines)
	}
}
