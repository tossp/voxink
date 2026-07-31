package asr

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tossp/voxink/internal/domain"
)

func TestDefaultRegistryKeepsVendorAndModelIdentitySeparate(t *testing.T) {
	registry := DefaultRegistry()
	mimo, ok := registry.Lookup(VendorMiMo)
	if !ok {
		t.Fatal("MiMo vendor is not registered")
	}
	want := []domain.ProviderKind{domain.ProviderMiMoASR, domain.ProviderMiMoV25}
	if !reflect.DeepEqual(mimo.Models, want) {
		t.Fatalf("MiMo models = %v, want %v", mimo.Models, want)
	}
}

func TestRouteValidate(t *testing.T) {
	registry := DefaultRegistry()
	tests := []struct {
		name    string
		route   Route
		wantErr bool
	}{
		{name: "primary only", route: Route{Primary: VendorMiMo}},
		{name: "different backup", route: Route{Primary: VendorMiMo, Backup: VendorMOSI}},
		{name: "missing primary", route: Route{}, wantErr: true},
		{name: "unknown primary", route: Route{Primary: "unknown"}, wantErr: true},
		{name: "same backup", route: Route{Primary: VendorMiMo, Backup: VendorMiMo}, wantErr: true},
		{name: "unknown backup", route: Route{Primary: VendorMiMo, Backup: "unknown"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.route.Validate(registry)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStageOneRouteIsFixedAndValid(t *testing.T) {
	route := StageOneRoute()
	want := ProviderRoute{
		Primary: RouteProvider{Vendor: VendorVolcengine, Model: domain.ProviderVolcengineV3},
		Backup:  RouteProvider{Vendor: VendorMiMo, Model: domain.ProviderMiMoASR},
	}
	if !reflect.DeepEqual(route, want) {
		t.Fatalf("StageOneRoute() = %+v, want %+v", route, want)
	}
	if err := ValidateStageOneRoute(route, DefaultRegistry()); err != nil {
		t.Fatalf("ValidateStageOneRoute() error = %v", err)
	}
}

func TestValidateStageOneRouteRejectsExcludedModels(t *testing.T) {
	registry := DefaultRegistry()
	tests := []struct {
		name  string
		route ProviderRoute
	}{
		{
			name: "Audio Understanding as backup",
			route: ProviderRoute{
				Primary: RouteProvider{Vendor: VendorVolcengine, Model: domain.ProviderVolcengineV3},
				Backup:  RouteProvider{Vendor: VendorMiMo, Model: domain.ProviderMiMoV25},
			},
		},
		{
			name: "MOSI model under MiMo",
			route: ProviderRoute{
				Primary: RouteProvider{Vendor: VendorVolcengine, Model: domain.ProviderVolcengineV3},
				Backup:  RouteProvider{Vendor: VendorMiMo, Model: domain.ProviderMOSI},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateStageOneRoute(tt.route, registry); err == nil {
				t.Fatal("ValidateStageOneRoute() error = nil, want fixed route failure")
			}
		})
	}
}

func TestProviderRouteValidateRequiresModelOwnership(t *testing.T) {
	route := ProviderRoute{
		Primary: RouteProvider{Vendor: VendorVolcengine, Model: domain.ProviderVolcengineV3},
		Backup:  RouteProvider{Vendor: VendorMiMo, Model: domain.ProviderMOSI},
	}
	err := route.Validate(DefaultRegistry())
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("Validate() error = %v, want model ownership failure", err)
	}
}

type stubTranscriber struct {
	text  string
	err   error
	calls int
	seen  [][]byte
}

func (s *stubTranscriber) Transcribe(_ context.Context, pcm []byte) (string, error) {
	s.calls++
	s.seen = append(s.seen, append([]byte(nil), pcm...))
	return s.text, s.err
}

func newTestFallback(t *testing.T, primary, backup SegmentTranscriber) *FallbackTranscriber {
	t.Helper()
	transcriber, err := NewFallbackTranscriber(
		Route{Primary: VendorMiMo, Backup: VendorMOSI},
		DefaultRegistry(),
		map[AsrVendor]SegmentTranscriber{VendorMiMo: primary, VendorMOSI: backup},
	)
	if err != nil {
		t.Fatalf("NewFallbackTranscriber() error = %v", err)
	}
	return transcriber
}

func TestFallbackPrimarySuccessDoesNotCallBackup(t *testing.T) {
	primary := &stubTranscriber{text: "primary"}
	backup := &stubTranscriber{text: "backup"}
	transcriber := newTestFallback(t, primary, backup)

	got, err := transcriber.Transcribe(context.Background(), []byte{1, 2})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if got != "primary" || primary.calls != 1 || backup.calls != 0 {
		t.Fatalf("got %q, primary calls %d, backup calls %d", got, primary.calls, backup.calls)
	}
}

func TestFallbackCallsBackupWithSamePCM(t *testing.T) {
	primary := &stubTranscriber{err: errors.New("primary broke")}
	backup := &stubTranscriber{text: "backup"}
	transcriber := newTestFallback(t, primary, backup)
	pcm := []byte{4, 3, 2, 1}

	got, err := transcriber.Transcribe(context.Background(), pcm)
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if got != "backup" || backup.calls != 1 {
		t.Fatalf("got %q, backup calls %d", got, backup.calls)
	}
	if !reflect.DeepEqual(primary.seen, backup.seen) {
		t.Fatalf("primary PCM %v differs from backup PCM %v", primary.seen, backup.seen)
	}
}

func TestFallbackReportsBothFailuresWithoutPCM(t *testing.T) {
	primary := &stubTranscriber{err: errors.New("primary broke")}
	backup := &stubTranscriber{err: errors.New("backup broke")}
	transcriber := newTestFallback(t, primary, backup)

	_, err := transcriber.Transcribe(context.Background(), []byte("secret-audio"))
	if err == nil {
		t.Fatal("Transcribe() error = nil, want failure")
	}
	message := err.Error()
	for _, want := range []string{"primary broke", "backup broke"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q does not contain %q", message, want)
		}
	}
	if strings.Contains(message, "secret-audio") {
		t.Fatalf("error contains PCM content: %q", message)
	}
}
