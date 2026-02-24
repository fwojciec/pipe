package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	osexec "os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/fwojciec/pipe"
)

type bashArgs struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // milliseconds
}

// BashTool returns the tool definition for the bash tool.
func BashTool() pipe.Tool {
	return pipe.Tool{
		Name: "bash",
		Description: fmt.Sprintf(
			"Execute a bash command. Output truncated to last %d lines or %dKB; "+
				"if truncated, full output saved to temp file readable with the read tool.",
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
					"description": "Timeout in milliseconds (default: 120000)"
				}
			},
			"required": ["command"]
		}`),
	}
}

const rollingBufSize = 2 * DefaultMaxBytes // 100KB rolling buffer

// ExecuteBash executes a bash command and returns the result with separate
// stdout/stderr, output sanitization, tail truncation, and file offloading.
func ExecuteBash(ctx context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	if a.Command == "" {
		return domainError("command is required"), nil
	}

	timeout := 120 * time.Second
	if a.Timeout > 0 {
		timeout = time.Duration(a.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := osexec.CommandContext(ctx, "bash", "-c", a.Command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return domainError(fmt.Sprintf("failed to create stdout pipe: %s", err)), nil
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return domainError(fmt.Sprintf("failed to create stderr pipe: %s", err)), nil
	}

	if err := cmd.Start(); err != nil {
		return domainError(fmt.Sprintf("failed to start command: %s", err)), nil
	}

	stdoutC := NewOutputCollector(int64(DefaultMaxBytes), rollingBufSize)
	stderrC := NewOutputCollector(int64(DefaultMaxBytes), rollingBufSize)
	defer stdoutC.Close()
	defer stderrC.Close()

	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go func() { _, _ = io.Copy(stdoutC, stdoutPipe); close(stdoutDone) }()
	go func() { _, _ = io.Copy(stderrC, stderrPipe); close(stderrDone) }()

	<-stdoutDone
	<-stderrDone
	waitErr := cmd.Wait()

	// Determine exit code.
	exitCode := 0
	isError := false
	if waitErr != nil {
		var exitErr *osexec.ExitError
		isRealExit := errors.As(waitErr, &exitErr) && exitErr.ExitCode() >= 0
		if !isRealExit && ctx.Err() != nil {
			return formatTimeoutResult(ctx.Err(), stdoutC, stderrC), nil
		}
		isError = true
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return formatResult(exitCode, isError, stdoutC, stderrC), nil
}

// processOutput sanitizes and truncates collector output. Returns the processed
// string and truncation metadata.
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

func formatTimeoutResult(ctxErr error, stdout, stderr *OutputCollector) *pipe.ToolResult {
	stdoutStr, stdoutTR := processOutput(stdout)
	stderrStr, stderrTR := processOutput(stderr)

	var b strings.Builder
	fmt.Fprintf(&b, "command timed out: %s\n", ctxErr)
	if stdoutStr != "" {
		fmt.Fprintf(&b, "\nstdout (partial):\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(&b, "\nstderr (partial):\n%s\n", stderrStr)
	}

	appendOffloadNotice(&b, "stdout", stdoutTR, stdout)
	appendOffloadNotice(&b, "stderr", stderrTR, stderr)

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
		IsError: true,
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
