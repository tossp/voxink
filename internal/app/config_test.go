package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tossp/voxink/internal/provider/mimo"
	"github.com/tossp/voxink/internal/provider/volcengine"
)

func TestLoadRuntimeConfigNewAndLegacyVolcAuth(t *testing.T) {
	newConfig, err := LoadRuntimeConfig(env(map[string]string{
		envVolcAPIKey: "new-secret", envVolcResourceID: "resource", envMiMoAPIKey: "mimo-secret",
	}))
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(new) error = %v", err)
	}
	if newConfig.volc.Auth.Mode != volcengine.AuthAPIKey || newConfig.volc.Auth.APIKey != "new-secret" {
		t.Fatalf("new Volc auth = %+v", newConfig.volc.Auth)
	}
	if newConfig.mimo.AuthMode != mimo.AuthAPIKey || newConfig.mimo.Language != mimo.LanguageAuto {
		t.Fatalf("MiMo config = %+v", newConfig.mimo)
	}

	legacyConfig, err := LoadRuntimeConfig(env(map[string]string{
		envVolcAppKey: "legacy-app", envVolcAccessKey: "legacy-access", envVolcResourceID: "resource",
	}))
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(legacy) error = %v", err)
	}
	if legacyConfig.volc.Auth.Mode != volcengine.AuthLegacy || legacyConfig.volc.Auth.AccessKey != "legacy-access" {
		t.Fatalf("legacy Volc auth = %+v", legacyConfig.volc.Auth)
	}
}

func TestLoadRuntimeConfigCredentialsAndReadLimitValidation(t *testing.T) {
	if _, err := LoadRuntimeConfig(env(nil)); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("missing credentials error = %v, want ErrNoCredentials", err)
	}
	if _, err := LoadRuntimeConfig(env(map[string]string{
		envVolcAPIKey: "secret", envVolcResourceID: "resource", envVolcReadLimit: "not-a-number",
	})); err == nil || !strings.Contains(err.Error(), envVolcReadLimit) {
		t.Fatalf("invalid read limit error = %v", err)
	}
	config, err := LoadRuntimeConfig(env(map[string]string{
		envVolcAPIKey: "secret", envVolcResourceID: "resource", envVolcReadLimit: "65536",
	}))
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	if config.volc.ReadLimit != 65536 || config.volcReadLimit != 65536 {
		t.Fatalf("read limit = %d / %d", config.volc.ReadLimit, config.volcReadLimit)
	}
	if _, err := LoadRuntimeConfig(env(map[string]string{
		envVolcAPIKey: "new", envVolcAppKey: "legacy", envVolcAccessKey: "legacy-access", envVolcResourceID: "resource",
	})); err == nil {
		t.Fatal("mixed new and legacy Volcengine credentials were accepted")
	}
}

func TestLoadRuntimeConfigEndpointOverridesAndRedaction(t *testing.T) {
	values := map[string]string{
		envVolcAPIKey: "volc-super-secret", envVolcResourceID: "resource-secret",
		envMiMoAPIKey: "mimo-super-secret", envVolcEndpoint: "ws://127.0.0.1:9001/live",
		envMiMoEndpoint: "http://127.0.0.1:9002/asr", envMiMoAuthMode: "bearer",
	}
	config, err := LoadRuntimeConfig(env(values))
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	if config.volc.Endpoint != values[envVolcEndpoint] || config.mimo.Endpoint != values[envMiMoEndpoint] {
		t.Fatalf("endpoint overrides were not retained")
	}
	if config.mimo.AuthMode != mimo.AuthBearer {
		t.Fatalf("MiMo auth mode = %q", config.mimo.AuthMode)
	}
	summary := fmt.Sprint(config)
	for _, secret := range []string{"volc-super-secret", "resource-secret", "mimo-super-secret", values[envVolcEndpoint], values[envMiMoEndpoint]} {
		if strings.Contains(summary, secret) {
			t.Fatalf("redacted summary leaked %q: %s", secret, summary)
		}
	}

	querySecret := "query-secret"
	_, err = LoadRuntimeConfig(env(map[string]string{
		envMiMoAPIKey: "key", envMiMoEndpoint: "http://localhost/asr?token=" + querySecret,
	}))
	if err == nil || strings.Contains(err.Error(), querySecret) {
		t.Fatalf("endpoint query validation error leaked secret: %v", err)
	}
}

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
