package exec

// WaitBackground exposes background process completion for tests.
func (e *BashExecutor) WaitBackground(pid int) <-chan struct{} {
	return e.bg.done(pid)
}
