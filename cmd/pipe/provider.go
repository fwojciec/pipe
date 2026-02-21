package main

import (
	"context"
	"fmt"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/anthropic"
	"github.com/fwojciec/pipe/gemini"
)

type providerConfig struct {
	name string
	key  string
}

// resolveConfig determines the provider name and API key from flags and env
// vars. Pure logic — no side effects.
func resolveConfig(providerFlag, apiKeyFlag, anthropicEnvKey, geminiEnvKey string) (providerConfig, error) {
	provider := providerFlag

	// Auto-detect from env vars if no flag.
	if provider == "" {
		hasAnthropic := anthropicEnvKey != ""
		hasGemini := geminiEnvKey != ""
		switch {
		case hasAnthropic && hasGemini:
			return providerConfig{}, fmt.Errorf("multiple API keys found (ANTHROPIC_API_KEY, GEMINI_API_KEY): use -provider flag to select")
		case hasAnthropic:
			provider = "anthropic"
		case hasGemini:
			provider = "gemini"
		default:
			return providerConfig{}, fmt.Errorf("no API key found: set ANTHROPIC_API_KEY or GEMINI_API_KEY (or use -provider and -api-key flags)")
		}
	}

	// Resolve API key: explicit flag overrides env var.
	key := apiKeyFlag
	switch provider {
	case "anthropic":
		if key == "" {
			key = anthropicEnvKey
		}
		if key == "" {
			return providerConfig{}, fmt.Errorf("ANTHROPIC_API_KEY not set (use -api-key flag or environment variable)")
		}
	case "gemini":
		if key == "" {
			key = geminiEnvKey
		}
		if key == "" {
			return providerConfig{}, fmt.Errorf("GEMINI_API_KEY not set (use -api-key flag or environment variable)")
		}
	default:
		return providerConfig{}, fmt.Errorf("unknown provider %q: must be \"anthropic\" or \"gemini\"", provider)
	}

	return providerConfig{name: provider, key: key}, nil
}

// resolveProvider selects and constructs the provider. All env var values are
// passed in as parameters — env is only read in main().
func resolveProvider(providerFlag, apiKeyFlag, anthropicEnvKey, geminiEnvKey string) (pipe.Provider, error) {
	cfg, err := resolveConfig(providerFlag, apiKeyFlag, anthropicEnvKey, geminiEnvKey)
	if err != nil {
		return nil, err
	}

	switch cfg.name {
	case "anthropic":
		return anthropic.New(cfg.key), nil
	case "gemini":
		// Use context.Background() for client construction — the genai SDK may
		// store this context for the client's lifetime. The signal context is
		// passed per-call via Stream(ctx, ...).
		client, err := gemini.New(context.Background(), cfg.key)
		if err != nil {
			return nil, fmt.Errorf("gemini: %w", err)
		}
		return client, nil
	default:
		// Defensive: resolveConfig validates the name, but guard against future drift.
		return nil, fmt.Errorf("unknown provider %q: must be \"anthropic\" or \"gemini\"", cfg.name)
	}
}
