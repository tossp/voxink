package settings

import (
	"testing"

	"github.com/tossp/voxink/internal/provider/mimo"
	"github.com/tossp/voxink/internal/provider/volcengine"
)

func TestResolvePrecedenceSettingsEnvironmentDefaults(t *testing.T) {
	document := EmptyDocument()
	for key, value := range map[Key]string{
		HotkeyKey:             "Alt+F12",
		VolcEndpointKey:       "wss://settings.example/volc",
		VolcReadLimitBytesKey: "2097152",
		MiMoEndpointKey:       "https://settings.example/mimo",
		MiMoAuthModeKey:       "bearer",
	} {
		if err := Set(&document, key, value); err != nil {
			t.Fatalf("Set(%s) error = %v", key, err)
		}
	}
	environment := map[string]string{
		"VOXINK_HOTKEY":                "Ctrl+A",
		"VOXINK_VOLC_ENDPOINT":         "wss://env.example/volc",
		"VOXINK_VOLC_READ_LIMIT_BYTES": "3145728",
		"VOXINK_MIMO_ENDPOINT":         "https://env.example/mimo",
		"VOXINK_MIMO_AUTH_MODE":        "api-key",
	}
	effective, err := Resolve(document, func(key string) string { return environment[key] })
	if err != nil {
		t.Fatalf("Resolve(settings) error = %v", err)
	}
	if effective.Hotkey.String() != "Alt+F12" || effective.VolcEndpoint != "wss://settings.example/volc" ||
		effective.VolcReadLimitBytes != 2097152 || effective.MiMoEndpoint != "https://settings.example/mimo" || effective.MiMoAuthMode != mimo.AuthBearer {
		t.Fatalf("settings precedence = %+v", effective)
	}

	effective, err = Resolve(EmptyDocument(), func(key string) string { return environment[key] })
	if err != nil {
		t.Fatalf("Resolve(env) error = %v", err)
	}
	if effective.Hotkey.String() != "Ctrl+A" || effective.VolcEndpoint != environment["VOXINK_VOLC_ENDPOINT"] ||
		effective.VolcReadLimitBytes != 3145728 || effective.MiMoEndpoint != environment["VOXINK_MIMO_ENDPOINT"] || effective.MiMoAuthMode != mimo.AuthAPIKey {
		t.Fatalf("environment precedence = %+v", effective)
	}

	effective, err = Resolve(EmptyDocument(), func(string) string { return "" })
	if err != nil {
		t.Fatalf("Resolve(defaults) error = %v", err)
	}
	if effective.Hotkey.String() != "Ctrl+Shift+Space" || effective.VolcEndpoint != volcengine.DefaultEndpoint ||
		effective.VolcReadLimitBytes != DefaultVolcReadLimitBytes || effective.MiMoEndpoint != mimo.DefaultEndpoint || effective.MiMoAuthMode != mimo.AuthAPIKey {
		t.Fatalf("defaults = %+v", effective)
	}
}

func TestSetRejectsInvalidValuesWithoutMutation(t *testing.T) {
	tests := []struct {
		key   Key
		value string
	}{
		{HotkeyKey, "Ctrl+Escape"},
		{VolcEndpointKey, "wss://user:token@example.test/asr"},
		{VolcEndpointKey, "wss://example.test/asr?token=canary"},
		{VolcEndpointKey, "https://example.test/asr"},
		{VolcReadLimitBytesKey, "1"},
		{VolcReadLimitBytesKey, "67108865"},
		{MiMoEndpointKey, "http://example.test/asr"},
		{MiMoEndpointKey, "https://example.test/asr#fragment"},
		{MiMoAuthModeKey, "basic"},
	}
	for _, test := range tests {
		document := EmptyDocument()
		if err := Set(&document, test.key, test.value); err == nil {
			t.Errorf("Set(%s, %q) error = nil", test.key, test.value)
		}
		if document != (Document{SchemaVersion: SchemaVersion}) {
			t.Errorf("Set(%s) mutated document: %+v", test.key, document)
		}
	}
}
