package main_test

import (
	"testing"

	. "github.com/fwojciec/pipe/cmd/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveConfig_ExplicitAnthropic(t *testing.T) {
	t.Parallel()
	name, key, err := ResolveConfigForTest("anthropic", "sk-test", "", "")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", name)
	assert.Equal(t, "sk-test", key)
}

func TestResolveConfig_ExplicitGemini(t *testing.T) {
	t.Parallel()
	name, key, err := ResolveConfigForTest("gemini", "gk-test", "", "")
	require.NoError(t, err)
	assert.Equal(t, "gemini", name)
	assert.Equal(t, "gk-test", key)
}

func TestResolveConfig_UnknownProvider(t *testing.T) {
	t.Parallel()
	_, _, err := ResolveConfigForTest("openai", "key", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestResolveConfig_NoKeysNoFlag(t *testing.T) {
	t.Parallel()
	_, _, err := ResolveConfigForTest("", "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no API key found")
}

func TestResolveConfig_BothKeysNoFlag(t *testing.T) {
	t.Parallel()
	_, _, err := ResolveConfigForTest("", "", "sk-ant", "gk-gem")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple API keys")
}

func TestResolveConfig_AutoDetectAnthropic(t *testing.T) {
	t.Parallel()
	name, key, err := ResolveConfigForTest("", "", "sk-ant", "")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", name)
	assert.Equal(t, "sk-ant", key)
}

func TestResolveConfig_AutoDetectGemini(t *testing.T) {
	t.Parallel()
	name, key, err := ResolveConfigForTest("", "", "", "gk-gem")
	require.NoError(t, err)
	assert.Equal(t, "gemini", name)
	assert.Equal(t, "gk-gem", key)
}

func TestResolveConfig_FlagKeyOverridesEnv(t *testing.T) {
	t.Parallel()
	name, key, err := ResolveConfigForTest("anthropic", "sk-flag", "sk-env", "")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", name)
	assert.Equal(t, "sk-flag", key)
}

func TestResolveConfig_ExplicitProviderMissingKey(t *testing.T) {
	t.Parallel()
	_, _, err := ResolveConfigForTest("anthropic", "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ANTHROPIC_API_KEY not set")
}

func TestResolveConfig_ExplicitGeminiMissingKey(t *testing.T) {
	t.Parallel()
	_, _, err := ResolveConfigForTest("gemini", "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GEMINI_API_KEY not set")
}
