package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/credential"
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
		envMiMoAPIKey: "mimo-secret",
	}))
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(legacy) error = %v", err)
	}
	if legacyConfig.volc.Auth.Mode != volcengine.AuthLegacy || legacyConfig.volc.Auth.AccessKey != "legacy-access" {
		t.Fatalf("legacy Volc auth = %+v", legacyConfig.volc.Auth)
	}
	if newConfig.route != asr.StageOneRoute() || legacyConfig.route != asr.StageOneRoute() {
		t.Fatalf("runtime routes = %+v / %+v", newConfig.route, legacyConfig.route)
	}
}

func TestLoadRuntimeConfigRequiresBothStageOneCredentialSets(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		want   error
	}{
		{name: "both missing", want: ErrNoCredentials},
		{
			name:   "Volcengine missing",
			values: map[string]string{envMiMoAPIKey: "mimo-secret"},
			want:   ErrMissingVolcengineCredentials,
		},
		{
			name:   "MiMo missing",
			values: map[string]string{envVolcAPIKey: "volc-secret", envVolcResourceID: "resource"},
			want:   ErrMissingMiMoCredentials,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadRuntimeConfig(env(tt.values))
			if !errors.Is(err, tt.want) {
				t.Fatalf("LoadRuntimeConfig() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestLoadRuntimeConfigMissingCredentialErrorsAreRedactedAndActionable(t *testing.T) {
	tests := []struct {
		name      string
		values    map[string]string
		secret    string
		actionEnv string
	}{
		{
			name:   "Volcengine missing",
			values: map[string]string{envMiMoAPIKey: "mimo-secret-value"},
			secret: "mimo-secret-value", actionEnv: envVolcAPIKey,
		},
		{
			name:   "MiMo missing",
			values: map[string]string{envVolcAPIKey: "volc-secret-value", envVolcResourceID: "resource-secret-value"},
			secret: "volc-secret-value", actionEnv: envMiMoAPIKey,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadRuntimeConfig(env(tt.values))
			if err == nil {
				t.Fatal("LoadRuntimeConfig() error = nil")
			}
			message := err.Error()
			if strings.Contains(message, tt.secret) {
				t.Fatalf("credential error leaked secret: %q", message)
			}
			if !strings.Contains(message, tt.actionEnv) {
				t.Fatalf("credential error %q does not name %s", message, tt.actionEnv)
			}
		})
	}
}

func TestLoadRuntimeConfigCredentialsAndReadLimitValidation(t *testing.T) {
	if _, err := LoadRuntimeConfig(env(map[string]string{
		envVolcAPIKey: "secret", envVolcResourceID: "resource", envMiMoAPIKey: "mimo-secret",
		envVolcReadLimit: "not-a-number",
	})); err == nil || !strings.Contains(err.Error(), envVolcReadLimit) {
		t.Fatalf("invalid read limit error = %v", err)
	}
	config, err := LoadRuntimeConfig(env(map[string]string{
		envVolcAPIKey: "secret", envVolcResourceID: "resource", envMiMoAPIKey: "mimo-secret",
		envVolcReadLimit: "65536",
	}))
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	if config.volc.ReadLimit != 65536 || config.volcReadLimit != 65536 {
		t.Fatalf("read limit = %d / %d", config.volc.ReadLimit, config.volcReadLimit)
	}
	if _, err := LoadRuntimeConfig(env(map[string]string{
		envVolcAPIKey: "new", envVolcAppKey: "legacy", envVolcAccessKey: "legacy-access", envVolcResourceID: "resource",
		envMiMoAPIKey: "mimo-secret",
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
		envVolcAPIKey: "volc", envVolcResourceID: "resource", envMiMoAPIKey: "key",
		envMiMoEndpoint: "http://localhost/asr?token=" + querySecret,
	}))
	if err == nil || strings.Contains(err.Error(), querySecret) {
		t.Fatalf("endpoint query validation error leaked secret: %v", err)
	}
}

func TestProviderSpecificConfigLoadersInspectOnlySelectedProvider(t *testing.T) {
	volcValues := map[string]string{envVolcAPIKey: "volc", envVolcResourceID: "resource"}
	volcGetenv := func(key string) string {
		if strings.HasPrefix(key, "VOXINK_MIMO_") {
			t.Fatalf("Volcengine loader inspected %s", key)
		}
		return volcValues[key]
	}
	if _, err := LoadVolcengineConfig(volcGetenv); err != nil {
		t.Fatalf("LoadVolcengineConfig() error = %v", err)
	}

	mimoValues := map[string]string{envMiMoAPIKey: "mimo"}
	mimoGetenv := func(key string) string {
		if strings.HasPrefix(key, "VOXINK_VOLC_") {
			t.Fatalf("MiMo loader inspected %s", key)
		}
		return mimoValues[key]
	}
	if _, err := LoadMiMoConfig(mimoGetenv); err != nil {
		t.Fatalf("LoadMiMoConfig() error = %v", err)
	}
}

func TestLoadRuntimeConfigUsesStoredCredentialsBeforeEnvironment(t *testing.T) {
	store := configStore{values: map[credential.Name]string{
		credential.VolcAPIKey:     "stored-volc",
		credential.VolcResourceID: "stored-resource",
		credential.MiMoAPIKey:     "stored-mimo",
	}}
	config, err := LoadRuntimeConfigWithCredentials(env(map[string]string{
		envVolcAPIKey: "env-volc", envVolcResourceID: "env-resource", envMiMoAPIKey: "env-mimo",
	}), store)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithCredentials() error = %v", err)
	}
	if config.volc.Auth.APIKey != "stored-volc" || config.volc.Auth.ResourceID != "stored-resource" || config.mimo.APIKey != "stored-mimo" {
		t.Fatalf("stored credentials were not preferred: volc=%+v mimo=%+v", config.volc.Auth, config.mimo)
	}
}

func TestLoadRuntimeConfigStoredLegacyVolcCombination(t *testing.T) {
	store := configStore{values: map[credential.Name]string{
		credential.VolcAppKey:     "stored-app",
		credential.VolcAccessKey:  "stored-access",
		credential.VolcResourceID: "stored-resource",
		credential.MiMoAPIKey:     "stored-mimo",
	}}
	config, err := LoadRuntimeConfigWithCredentials(env(nil), store)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithCredentials() error = %v", err)
	}
	if config.volc.Auth.Mode != volcengine.AuthLegacy || config.volc.Auth.AppKey != "stored-app" || config.volc.Auth.AccessKey != "stored-access" {
		t.Fatalf("stored legacy auth = %+v", config.volc.Auth)
	}
}

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

type configStore struct{ values map[credential.Name]string }

func (s configStore) Read(name credential.Name) ([]byte, error) {
	value, ok := s.values[name]
	if !ok {
		return nil, credential.ErrNotFound
	}
	return []byte(value), nil
}
func (configStore) Write(credential.Name, []byte) error { return credential.ErrStorage }
func (configStore) Delete(credential.Name) error        { return credential.ErrStorage }
