// Command pipe is a minimal agentic coding harness.
//
// Usage:
//
//	ANTHROPIC_API_KEY=sk-... pipe [flags]
//	GEMINI_API_KEY=gk-...   pipe [flags]
//
// Flags:
//
//	-provider string     Provider: anthropic, gemini (auto-detected from env vars if omitted)
//	-model string        Model ID (default: provider default)
//	-session string      Path to session file to resume
//	-system-prompt string Path to system prompt file (default: .pipe/prompt.md)
//	-api-key string      API key (overrides provider's env var)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	pipejson "github.com/fwojciec/pipe/json"
)

const defaultPromptPath = ".pipe/prompt.md"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "pipe: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse flags.
	var (
		model        = flag.String("model", "", "Model ID (provider-specific)")
		sessionPath  = flag.String("session", "", "Path to session file to resume")
		promptPath   = flag.String("system-prompt", defaultPromptPath, "Path to system prompt file")
		providerFlag = flag.String("provider", "", "Provider: anthropic, gemini (auto-detected from env vars if omitted)")
		apiKey       = flag.String("api-key", "", "API key (overrides provider's env var)")
	)
	flag.Parse()

	// Handle OS signals for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Resolve provider. Env vars are read here and passed as values.
	provider, err := resolveProvider(*providerFlag, *apiKey,
		os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("GEMINI_API_KEY"))
	if err != nil {
		return err
	}

	// Load or create session.
	session, err := loadOrCreateSession(*sessionPath, *promptPath)
	if err != nil {
		return err
	}

	// Create tool executor and get tool definitions.
	exec := &executor{}
	toolDefs := tools()

	// Create agent loop.
	loop := pipe.NewLoop(provider, exec)

	// Build agent function closure for the TUI.
	modelID := *model
	agentFn := func(ctx context.Context, s *pipe.Session, onEvent func(pipe.Event)) error {
		opts := []pipe.RunOption{pipe.WithEventHandler(onEvent)}
		if modelID != "" {
			opts = append(opts, pipe.WithModel(modelID))
		}
		return loop.Run(ctx, s, toolDefs, opts...)
	}

	// Create and run TUI.
	theme := pipe.DefaultTheme()
	config := bt.Config{
		WorkDir:   workDir(),
		GitBranch: gitBranch(),
		ModelName: modelID,
	}
	tuiModel := bt.New(agentFn, &session, theme, config)

	if err := bt.Run(ctx, tuiModel); err != nil {
		return fmt.Errorf("TUI: %w", err)
	}

	// Save session on exit.
	if *sessionPath != "" {
		if err := pipejson.Save(*sessionPath, session); err != nil {
			return fmt.Errorf("save session: %w", err)
		}
	} else if len(session.Messages) > 0 {
		// Auto-save to default location.
		savePath := defaultSessionPath(session.ID)
		if err := pipejson.Save(savePath, session); err != nil {
			return fmt.Errorf("auto-save session: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Session saved to %s\n", savePath)
	}

	return nil
}

func loadOrCreateSession(sessionPath, promptPath string) (pipe.Session, error) {
	// Load existing session if path provided.
	if sessionPath != "" {
		s, err := pipejson.Load(sessionPath)
		if err != nil {
			return pipe.Session{}, fmt.Errorf("load session: %w", err)
		}
		return s, nil
	}

	// Load system prompt. Tolerate missing default; fail on all other errors.
	systemPrompt := "You are a helpful coding assistant."
	data, err := os.ReadFile(promptPath)
	switch {
	case err == nil:
		systemPrompt = string(data)
	case errors.Is(err, os.ErrNotExist) && promptPath == defaultPromptPath:
		// Default prompt file doesn't exist; use built-in default.
	default:
		return pipe.Session{}, fmt.Errorf("read system prompt: %w", err)
	}

	// Create new session.
	now := time.Now()
	return pipe.Session{
		ID:           fmt.Sprintf("%d", now.UnixNano()),
		SystemPrompt: systemPrompt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func defaultSessionPath(id string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".pipe", "sessions", id+".json")
}

func workDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return dir
	}
	if rel, ok := strings.CutPrefix(dir, home); ok {
		return "~" + rel
	}
	return dir
}

func gitBranch() string {
	// Walk up from cwd looking for a .git entry to avoid spawning git
	// outside repositories (saves ~50-100ms startup latency).
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	found := false
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			found = true
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if !found {
		return ""
	}
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
