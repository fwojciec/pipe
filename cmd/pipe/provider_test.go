package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProvider_ExplicitAnthropic(t *testing.T) {
	t.Parallel()
	p, err := resolveProvider(context.Background(), "anthropic", "sk-test", "", "")
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestResolveProvider_ExplicitGemini(t *testing.T) {
	t.Parallel()
	p, err := resolveProvider(context.Background(), "gemini", "gk-test", "", "")
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestResolveProvider_UnknownProvider(t *testing.T) {
	t.Parallel()
	_, err := resolveProvider(context.Background(), "openai", "key", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestResolveProvider_NoKeysNoFlag(t *testing.T) {
	t.Parallel()
	_, err := resolveProvider(context.Background(), "", "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no API key found")
}

func TestResolveProvider_BothKeysNoFlag(t *testing.T) {
	t.Parallel()
	_, err := resolveProvider(context.Background(), "", "", "sk-ant", "gk-gem")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple API keys")
}

func TestResolveProvider_AutoDetectAnthropic(t *testing.T) {
	t.Parallel()
	p, err := resolveProvider(context.Background(), "", "", "sk-ant", "")
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestResolveProvider_AutoDetectGemini(t *testing.T) {
	t.Parallel()
	p, err := resolveProvider(context.Background(), "", "", "", "gk-gem")
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestResolveProvider_FlagKeyOverridesEnv(t *testing.T) {
	t.Parallel()
	// -api-key flag overrides the env var for the selected provider.
	p, err := resolveProvider(context.Background(), "anthropic", "sk-flag", "sk-env", "")
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestResolveProvider_ExplicitProviderMissingKey(t *testing.T) {
	t.Parallel()
	_, err := resolveProvider(context.Background(), "anthropic", "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ANTHROPIC_API_KEY not set")
}

func TestResolveProvider_ExplicitGeminiMissingKey(t *testing.T) {
	t.Parallel()
	_, err := resolveProvider(context.Background(), "gemini", "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GEMINI_API_KEY not set")
}
