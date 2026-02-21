package main

import (
	"context"
	"fmt"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/anthropic"
	"github.com/fwojciec/pipe/gemini"
)

// resolveProvider selects and constructs the provider. All env var values are
// passed in as parameters — env is only read in main().
func resolveProvider(ctx context.Context, providerFlag, apiKeyFlag, anthropicEnvKey, geminiEnvKey string) (pipe.Provider, error) {
	provider := providerFlag

	// Auto-detect from env vars if no flag.
	if provider == "" {
		hasAnthropic := anthropicEnvKey != ""
		hasGemini := geminiEnvKey != ""
		switch {
		case hasAnthropic && hasGemini:
			return nil, fmt.Errorf("multiple API keys found (ANTHROPIC_API_KEY, GEMINI_API_KEY): use -provider flag to select")
		case hasAnthropic:
			provider = "anthropic"
		case hasGemini:
			provider = "gemini"
		default:
			return nil, fmt.Errorf("no API key found: set ANTHROPIC_API_KEY or GEMINI_API_KEY (or use -provider and -api-key flags)")
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
			return nil, fmt.Errorf("ANTHROPIC_API_KEY not set (use -api-key flag or environment variable)")
		}
		return anthropic.New(key), nil
	case "gemini":
		if key == "" {
			key = geminiEnvKey
		}
		if key == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY not set (use -api-key flag or environment variable)")
		}
		client, err := gemini.New(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("gemini: %w", err)
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unknown provider %q: must be \"anthropic\" or \"gemini\"", provider)
	}
}
