package app

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/provider/mimo"
	"github.com/tossp/voxink/internal/provider/volcengine"
)

const (
	// DefaultVolcengineReadLimit is a local defensive cap, not a provider-published maximum.
	DefaultVolcengineReadLimit int64 = 1024 * 1024

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

// RuntimeConfig contains environment-derived provider construction inputs.
// Its String method intentionally exposes only non-secret availability metadata.
type RuntimeConfig struct {
	route         asr.ProviderRoute
	volc          *volcengine.Config
	mimo          *mimo.Config
	volcOverride  bool
	mimoOverride  bool
	volcReadLimit int64
}

// LoadRuntimeConfig reads stage-one settings without persistence or logging.
func LoadRuntimeConfig(getenv func(string) string) (RuntimeConfig, error) {
	volcConfig, volcOverride, readLimit, err := loadVolcengineConfig(getenv)
	if err != nil {
		return RuntimeConfig{}, err
	}
	mimoConfig, mimoOverride, err := loadMiMoConfig(getenv)
	if err != nil {
		return RuntimeConfig{}, err
	}

	config := RuntimeConfig{
		route:        asr.StageOneRoute(),
		volc:         volcConfig,
		mimo:         mimoConfig,
		volcOverride: volcOverride, mimoOverride: mimoOverride, volcReadLimit: readLimit,
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
	config, _, _, err := loadVolcengineConfig(getenv)
	if err != nil {
		return volcengine.Config{}, err
	}
	if config == nil {
		return volcengine.Config{}, ErrMissingVolcengineCredentials
	}
	return *config, nil
}

func loadVolcengineConfig(getenv func(string) string) (*volcengine.Config, bool, int64, error) {
	readLimit := DefaultVolcengineReadLimit
	if raw := strings.TrimSpace(getenv(envVolcReadLimit)); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			return nil, false, 0, fmt.Errorf("%s must be a positive integer", envVolcReadLimit)
		}
		readLimit = parsed
	}
	endpoint, override, err := endpointOverride(getenv(envVolcEndpoint), "Volcengine", "ws", "wss")
	if err != nil {
		return nil, false, 0, err
	}
	apiKey := getenv(envVolcAPIKey)
	resourceID := getenv(envVolcResourceID)
	appKey := getenv(envVolcAppKey)
	accessKey := getenv(envVolcAccessKey)
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
	config, _, err := loadMiMoConfig(getenv)
	if err != nil {
		return mimo.Config{}, err
	}
	if config == nil {
		return mimo.Config{}, ErrMissingMiMoCredentials
	}
	return *config, nil
}

func loadMiMoConfig(getenv func(string) string) (*mimo.Config, bool, error) {
	endpoint, override, err := endpointOverride(getenv(envMiMoEndpoint), "MiMo", "http", "https")
	if err != nil {
		return nil, false, err
	}
	key := getenv(envMiMoAPIKey)
	if key == "" {
		return nil, override, nil
	}
	authMode := mimo.AuthAPIKey
	switch strings.TrimSpace(getenv(envMiMoAuthMode)) {
	case "", string(mimo.AuthAPIKey):
	case string(mimo.AuthBearer):
		authMode = mimo.AuthBearer
	default:
		return nil, false, fmt.Errorf("%s must be api-key or bearer", envMiMoAuthMode)
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

func endpointOverride(raw, provider string, schemes ...string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("invalid %s endpoint override", provider)
	}
	allowed := false
	for _, scheme := range schemes {
		allowed = allowed || parsed.Scheme == scheme
	}
	if !allowed {
		return "", false, fmt.Errorf("invalid %s endpoint override scheme", provider)
	}
	return raw, true, nil
}

// String returns a redacted configuration summary.
func (c RuntimeConfig) String() string {
	return fmt.Sprintf(
		"RuntimeConfig{Volcengine:%t MiMo:%t VolcEndpointOverride:%t MiMoEndpointOverride:%t VolcReadLimit:%d}",
		c.volc != nil, c.mimo != nil, c.volcOverride, c.mimoOverride, c.volcReadLimit,
	)
}

// HasVolcengine reports whether the primary live route is configured.
func (c RuntimeConfig) HasVolcengine() bool { return c.volc != nil }

// HasMiMo reports whether the batch fallback route is configured.
func (c RuntimeConfig) HasMiMo() bool { return c.mimo != nil }
