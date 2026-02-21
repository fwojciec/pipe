package main

// ResolveConfigForTest exposes resolveConfig for external tests, returning
// the resolved provider name and key.
func ResolveConfigForTest(providerFlag, apiKeyFlag, anthropicEnvKey, geminiEnvKey string) (name, key string, err error) {
	cfg, err := resolveConfig(providerFlag, apiKeyFlag, anthropicEnvKey, geminiEnvKey)
	if err != nil {
		return "", "", err
	}
	return cfg.name, cfg.key, nil
}
