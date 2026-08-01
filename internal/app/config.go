package app

import (
	"errors"
	"fmt"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/credential"
	platformwindows "github.com/tossp/voxink/internal/platform/windows"
	"github.com/tossp/voxink/internal/provider/mimo"
	"github.com/tossp/voxink/internal/provider/volcengine"
	"github.com/tossp/voxink/internal/settings"
)

const (
	// DefaultVolcengineReadLimit is a local defensive cap, not a provider-published maximum.
	DefaultVolcengineReadLimit int64 = settings.DefaultVolcReadLimitBytes

	envVolcAPIKey     = "VOXINK_VOLC_API_KEY"
	envVolcResourceID = "VOXINK_VOLC_RESOURCE_ID"
	envVolcAppKey     = "VOXINK_VOLC_APP_KEY"
	envVolcAccessKey  = "VOXINK_VOLC_ACCESS_KEY"
	envVolcEndpoint   = "VOXINK_VOLC_ENDPOINT"
	envVolcReadLimit  = "VOXINK_VOLC_READ_LIMIT_BYTES"
	envMiMoAPIKey     = "VOXINK_MIMO_API_KEY"
	envMiMoAuthMode   = "VOXINK_MIMO_AUTH_MODE"
	envMiMoEndpoint   = "VOXINK_MIMO_ENDPOINT"
)

// RuntimeConfig contains resolved non-sensitive settings and provider construction inputs.
// Its String method intentionally exposes only non-secret availability metadata.
type RuntimeConfig struct {
	route        asr.ProviderRoute
	volc         *volcengine.Config
	mimo         *mimo.Config
	volcOverride bool
	mimoOverride bool
	hotkey       platformwindows.Hotkey
}

// LoadRuntimeConfig reads stage-one settings without persistence or logging.
func LoadRuntimeConfig(getenv func(string) string) (RuntimeConfig, error) {
	return LoadRuntimeConfigWithCredentials(getenv, nil)
}

// LoadRuntimeConfigWithCredentials reads Provider credentials from store before
// falling back to the existing environment variables.
func LoadRuntimeConfigWithCredentials(getenv func(string) string, store credential.Store) (RuntimeConfig, error) {
	return LoadRuntimeConfigWithSettings(getenv, store, nil)
}

// LoadRuntimeConfigWithSettings applies persisted non-sensitive settings before
// environment/default fallback while retaining credential-store precedence.
func LoadRuntimeConfigWithSettings(getenv func(string) string, store credential.Store, loader settings.Loader) (RuntimeConfig, error) {
	document, err := settings.Load(loader)
	if err != nil {
		return RuntimeConfig{}, err
	}
	effective, err := settings.Resolve(document, getenv)
	if err != nil {
		return RuntimeConfig{}, err
	}
	resolve := credential.Resolver{Store: store, Getenv: getenv}.Get
	volcConfig, volcOverride, _, err := loadVolcengineConfig(effective.VolcEndpoint, effective.VolcReadLimitBytes, resolve)
	if err != nil {
		return RuntimeConfig{}, err
	}
	mimoConfig, mimoOverride, err := loadMiMoConfig(effective.MiMoEndpoint, effective.MiMoAuthMode, resolve)
	if err != nil {
		return RuntimeConfig{}, err
	}

	config := RuntimeConfig{
		route:        asr.StageOneRoute(),
		volc:         volcConfig,
		mimo:         mimoConfig,
		volcOverride: volcOverride, mimoOverride: mimoOverride,
		hotkey: effective.Hotkey,
	}
	if err := asr.ValidateStageOneRoute(config.route, asr.DefaultRegistry()); err != nil {
		return RuntimeConfig{}, fmt.Errorf("validate fixed stage-one ASR route: %w", err)
	}
	if err := config.validateStageOne(); err != nil {
		return RuntimeConfig{}, err
	}
	return config, nil
}

// LoadVolcengineConfig reads and validates only the existing Volcengine
// environment variables. It does not inspect MiMo configuration.
func LoadVolcengineConfig(getenv func(string) string) (volcengine.Config, error) {
	return LoadVolcengineConfigWithCredentials(getenv, nil)
}

// LoadVolcengineConfigWithCredentials loads only Volcengine configuration with
// Credential Manager precedence over environment variables.
func LoadVolcengineConfigWithCredentials(getenv func(string) string, store credential.Store) (volcengine.Config, error) {
	return LoadVolcengineConfigWithSettings(getenv, store, nil)
}

// LoadVolcengineConfigWithSettings applies the same persisted override used by normal startup.
func LoadVolcengineConfigWithSettings(getenv func(string) string, store credential.Store, loader settings.Loader) (volcengine.Config, error) {
	document, err := settings.Load(loader)
	if err != nil {
		return volcengine.Config{}, err
	}
	endpoint, readLimit, err := settings.ResolveVolc(document, getenv)
	if err != nil {
		return volcengine.Config{}, err
	}
	config, _, _, err := loadVolcengineConfig(endpoint, readLimit, credential.Resolver{Store: store, Getenv: getenv}.Get)
	if err != nil {
		return volcengine.Config{}, err
	}
	if config == nil {
		return volcengine.Config{}, ErrMissingVolcengineCredentials
	}
	return *config, nil
}

func loadVolcengineConfig(endpoint string, readLimit int64, resolve func(credential.Name) (string, error)) (*volcengine.Config, bool, int64, error) {
	override := endpoint != volcengine.DefaultEndpoint
	apiKey, err := resolve(credential.VolcAPIKey)
	if err != nil {
		return nil, false, 0, err
	}
	resourceID, err := resolve(credential.VolcResourceID)
	if err != nil {
		return nil, false, 0, err
	}
	appKey, err := resolve(credential.VolcAppKey)
	if err != nil {
		return nil, false, 0, err
	}
	accessKey, err := resolve(credential.VolcAccessKey)
	if err != nil {
		return nil, false, 0, err
	}
	if apiKey != "" && (appKey != "" || accessKey != "") {
		return nil, false, 0, fmt.Errorf("Volcengine new and legacy credentials are mutually exclusive")
	}
	if apiKey != "" {
		if resourceID == "" {
			return nil, false, 0, fmt.Errorf("%s is required with %s", envVolcResourceID, envVolcAPIKey)
		}
		return newVolcConfig(endpoint, readLimit, volcengine.AuthConfig{
			Mode: volcengine.AuthAPIKey, APIKey: apiKey, ResourceID: resourceID,
		}), override, readLimit, nil
	} else if appKey != "" || accessKey != "" {
		if appKey == "" || accessKey == "" || resourceID == "" {
			return nil, false, 0, fmt.Errorf("legacy Volcengine auth requires app key, access key, and resource ID")
		}
		return newVolcConfig(endpoint, readLimit, volcengine.AuthConfig{
			Mode: volcengine.AuthLegacy, AppKey: appKey, AccessKey: accessKey, ResourceID: resourceID,
		}), override, readLimit, nil
	} else if resourceID != "" {
		return nil, false, 0, fmt.Errorf("Volcengine resource ID was set without credentials")
	}
	return nil, override, readLimit, nil
}

// LoadMiMoConfig reads and validates only the existing MiMo environment
// variables. It does not inspect Volcengine configuration.
func LoadMiMoConfig(getenv func(string) string) (mimo.Config, error) {
	return LoadMiMoConfigWithCredentials(getenv, nil)
}

// LoadMiMoConfigWithCredentials loads only MiMo configuration with Credential
// Manager precedence over the environment variable.
func LoadMiMoConfigWithCredentials(getenv func(string) string, store credential.Store) (mimo.Config, error) {
	return LoadMiMoConfigWithSettings(getenv, store, nil)
}

// LoadMiMoConfigWithSettings applies the same persisted override used by normal startup.
func LoadMiMoConfigWithSettings(getenv func(string) string, store credential.Store, loader settings.Loader) (mimo.Config, error) {
	document, err := settings.Load(loader)
	if err != nil {
		return mimo.Config{}, err
	}
	endpoint, authMode, err := settings.ResolveMiMo(document, getenv)
	if err != nil {
		return mimo.Config{}, err
	}
	config, _, err := loadMiMoConfig(endpoint, authMode, credential.Resolver{Store: store, Getenv: getenv}.Get)
	if err != nil {
		return mimo.Config{}, err
	}
	if config == nil {
		return mimo.Config{}, ErrMissingMiMoCredentials
	}
	return *config, nil
}

func loadMiMoConfig(endpoint string, authMode mimo.AuthMode, resolve func(credential.Name) (string, error)) (*mimo.Config, bool, error) {
	override := endpoint != mimo.DefaultEndpoint
	key, err := resolve(credential.MiMoAPIKey)
	if err != nil {
		return nil, false, err
	}
	if key == "" {
		return nil, override, nil
	}
	return &mimo.Config{
		Endpoint: endpoint, AuthMode: authMode, APIKey: key, Language: mimo.LanguageAuto,
	}, override, nil
}

func (c RuntimeConfig) validateStageOne() error {
	if err := asr.ValidateStageOneRoute(c.route, asr.DefaultRegistry()); err != nil {
		return fmt.Errorf("validate runtime ASR route: %w", err)
	}
	if c.volc == nil && c.mimo == nil {
		return errors.Join(ErrNoCredentials, ErrMissingVolcengineCredentials, ErrMissingMiMoCredentials)
	}
	if c.volc == nil {
		return ErrMissingVolcengineCredentials
	}
	if c.mimo == nil {
		return ErrMissingMiMoCredentials
	}
	return nil
}

func newVolcConfig(endpoint string, readLimit int64, auth volcengine.AuthConfig) *volcengine.Config {
	return &volcengine.Config{
		Endpoint: endpoint, Auth: auth, ReadLimit: readLimit,
		Request: volcengine.RequestConfig{EnableITN: true, EnablePunc: true},
	}
}

// String returns a redacted configuration summary.
func (c RuntimeConfig) String() string {
	return fmt.Sprintf(
		"RuntimeConfig{Volcengine:%t MiMo:%t VolcEndpointOverride:%t MiMoEndpointOverride:%t}",
		c.volc != nil, c.mimo != nil, c.volcOverride, c.mimoOverride,
	)
}

// HasVolcengine reports whether the primary live route is configured.
func (c RuntimeConfig) HasVolcengine() bool { return c.volc != nil }

// HasMiMo reports whether the batch fallback route is configured.
func (c RuntimeConfig) HasMiMo() bool { return c.mimo != nil }
