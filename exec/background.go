package exec

import (
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fwojciec/pipe"
)

// BackgroundProcess tracks a process that was auto-backgrounded on timeout.
type BackgroundProcess struct {
	cmd        *osexec.Cmd
	stdout     *OutputCollector
	stderr     *OutputCollector
	waitCh     <-chan error
	stdoutDone <-chan struct{}
	stderrDone <-chan struct{}

	mu       sync.Mutex
	done     bool
	exitCode int
}

// watch waits for the background process to complete and records its exit code.
// Run as a goroutine.
func (bp *BackgroundProcess) watch() {
	waitErr := <-bp.waitCh
	<-bp.stdoutDone
	<-bp.stderrDone
	bp.stdout.Close()
	bp.stderr.Close()

	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.done = true
	if waitErr != nil {
		if exitErr, ok := waitErr.(*osexec.ExitError); ok {
			bp.exitCode = exitErr.ExitCode()
		} else {
			bp.exitCode = -1
		}
	}
}

// BackgroundRegistry tracks auto-backgrounded processes.
type BackgroundRegistry struct {
	mu        sync.Mutex
	processes map[int]*BackgroundProcess
}

// NewBackgroundRegistry creates an empty registry.
func NewBackgroundRegistry() *BackgroundRegistry {
	return &BackgroundRegistry{processes: make(map[int]*BackgroundProcess)}
}

// Register adds a background process.
func (r *BackgroundRegistry) Register(pid int, bp *BackgroundProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processes[pid] = bp
}

// Check returns the current status and output of a background process.
func (r *BackgroundRegistry) Check(pid int) (*pipe.ToolResult, error) {
	r.mu.Lock()
	bp, ok := r.processes[pid]
	r.mu.Unlock()

	if !ok {
		return domainError(fmt.Sprintf("no background process with pid %d", pid)), nil
	}

	bp.mu.Lock()
	done := bp.done
	exitCode := bp.exitCode
	bp.mu.Unlock()

	stdoutStr, stdoutTR := processOutput(bp.stdout)
	stderrStr, stderrTR := processOutput(bp.stderr)

	var b strings.Builder
	if done {
		fmt.Fprintf(&b, "[Process %d exited with code %d.\n", pid, exitCode)
	} else {
		fmt.Fprintf(&b, "[Process %d still running.\n", pid)
	}
	if stdoutStr != "" {
		fmt.Fprintf(&b, "\nstdout:\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(&b, "\nstderr:\n%s\n", stderrStr)
	}
	appendOffloadNotice(&b, "stdout", stdoutTR, bp.stdout)
	appendOffloadNotice(&b, "stderr", stderrTR, bp.stderr)
	b.WriteString("]")

	// Remove completed processes from registry to prevent leaking memory.
	if done {
		cleanupCollectorFiles(bp.stdout, bp.stderr)
		r.mu.Lock()
		delete(r.processes, pid)
		r.mu.Unlock()
	}

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
		IsError: done && exitCode != 0,
	}, nil
}

// Kill terminates a background process and returns its final output.
func (r *BackgroundRegistry) Kill(pid int) (*pipe.ToolResult, error) {
	r.mu.Lock()
	bp, ok := r.processes[pid]
	r.mu.Unlock()

	if !ok {
		return domainError(fmt.Sprintf("no background process with pid %d", pid)), nil
	}

	bp.mu.Lock()
	done := bp.done
	bp.mu.Unlock()

	if !done {
		_ = syscall.Kill(-bp.cmd.Process.Pid, syscall.SIGKILL)
		// Wait for watch goroutine to finish with bounded timeout.
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			bp.mu.Lock()
			if bp.done {
				bp.mu.Unlock()
				break
			}
			bp.mu.Unlock()
			select {
			case <-deadline.C:
				return domainError(fmt.Sprintf("timeout waiting for process %d to exit after kill", pid)), nil
			case <-tick.C:
			}
		}
	}

	// Build result before removing from registry.
	stdoutStr, stdoutTR := processOutput(bp.stdout)
	stderrStr, stderrTR := processOutput(bp.stderr)

	var b strings.Builder
	if done {
		fmt.Fprintf(&b, "[Process %d already exited.\n", pid)
	} else {
		fmt.Fprintf(&b, "[Process %d killed.\n", pid)
	}
	if stdoutStr != "" {
		fmt.Fprintf(&b, "\nstdout:\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(&b, "\nstderr:\n%s\n", stderrStr)
	}
	appendOffloadNotice(&b, "stdout", stdoutTR, bp.stdout)
	appendOffloadNotice(&b, "stderr", stderrTR, bp.stderr)
	b.WriteString("]")

	// Clean up temp files and remove from registry.
	cleanupCollectorFiles(bp.stdout, bp.stderr)
	r.mu.Lock()
	delete(r.processes, pid)
	r.mu.Unlock()

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
		IsError: false,
	}, nil
}

// cleanupCollectorFiles removes temp files created by output collectors.
func cleanupCollectorFiles(collectors ...*OutputCollector) {
	for _, c := range collectors {
		if p := c.FilePath(); p != "" {
			os.Remove(p)
		}
	}
}
