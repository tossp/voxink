package windows

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTrayRuntimeAddUpdateActionsAndIdempotentDelete(t *testing.T) {
	backend := &fakeTrayBackend{popupCommand: trayMenuToggleID}
	tray := newTrayRuntime(backend, 42)
	if err := tray.Open(); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if backend.adds != 1 || backend.hwnd != 42 || backend.tooltips[0] != "VoxInk - Idle" {
		t.Fatalf("add = (%d, %d, %q)", backend.adds, backend.hwnd, backend.tooltips)
	}

	secret := "recognized-canary credential-canary"
	if err := tray.Update(View{Status: ViewListening, Partial: secret, Error: secret, Notice: secret}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := backend.tooltips[len(backend.tooltips)-1]; got != "VoxInk - Capturing" || strings.Contains(got, secret) {
		t.Fatalf("capturing tooltip = %q", got)
	}
	if action, err := tray.HandleNotification(trayLeftDoubleClick); err != nil || action != trayActionToggle {
		t.Fatalf("double-click = (%v, %v)", action, err)
	}
	if action, err := tray.HandleNotification(trayRightButtonUp); err != nil || action != trayActionToggle {
		t.Fatalf("toggle menu = (%v, %v)", action, err)
	}
	if backend.toggleText != "停止" {
		t.Fatalf("capturing toggle text = %q", backend.toggleText)
	}

	backend.popupCommand = trayMenuExitID
	if err := tray.Update(View{Status: ViewTranscribing, Final: secret}); err != nil {
		t.Fatalf("transcribing Update() error = %v", err)
	}
	if action, err := tray.HandleNotification(trayRightButtonUp); err != nil || action != trayActionExit {
		t.Fatalf("exit menu = (%v, %v)", action, err)
	}
	if backend.toggleText != "开始" || backend.tooltips[len(backend.tooltips)-1] != "VoxInk - Transcribing" {
		t.Fatalf("transcribing menu/tooltip = (%q, %q)", backend.toggleText, backend.tooltips[len(backend.tooltips)-1])
	}
	if err := tray.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := tray.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if backend.deletes != 1 {
		t.Fatalf("Delete calls = %d, want 1", backend.deletes)
	}
}

func TestPopupTrayMenuAlwaysDestroysExactlyTwoItemMenu(t *testing.T) {
	api := &fakeTrayMenuAPI{command: trayMenuExitID}
	command, err := popupTrayMenu(api, 9, "开始")
	if err != nil || command != trayMenuExitID {
		t.Fatalf("popupTrayMenu() = (%d, %v)", command, err)
	}
	want := []fakeMenuItem{{trayMenuToggleID, "开始"}, {trayMenuExitID, "退出"}}
	if !reflect.DeepEqual(api.items, want) || api.destroyed != 1 {
		t.Fatalf("items/destroyed = (%v, %d), want (%v, 1)", api.items, api.destroyed, want)
	}

	appendErr := errors.New("append failed")
	api = &fakeTrayMenuAPI{appendErr: appendErr}
	if _, err := popupTrayMenu(api, 9, "开始"); !errors.Is(err, appendErr) {
		t.Fatalf("append error = %v", err)
	}
	if api.destroyed != 1 {
		t.Fatalf("Destroy calls after append failure = %d, want 1", api.destroyed)
	}
}

func TestExitIngressIsNonBlocking(t *testing.T) {
	overlay := NewOverlay()
	overlay.emitExit()
	done := make(chan struct{})
	go func() {
		overlay.emitExit()
		close(done)
	}()
	<-done
	select {
	case <-overlay.Exits():
	default:
		t.Fatal("first exit was not retained")
	}
}

type fakeTrayBackend struct {
	adds         int
	deletes      int
	hwnd         uintptr
	tooltips     []string
	popupCommand uint32
	toggleText   string
}

func (b *fakeTrayBackend) Add(hwnd uintptr, tooltip string) error {
	b.adds++
	b.hwnd = hwnd
	b.tooltips = append(b.tooltips, tooltip)
	return nil
}

func (b *fakeTrayBackend) Update(tooltip string) error {
	b.tooltips = append(b.tooltips, tooltip)
	return nil
}

func (b *fakeTrayBackend) Popup(_ uintptr, toggleText string) (uint32, error) {
	b.toggleText = toggleText
	return b.popupCommand, nil
}

func (b *fakeTrayBackend) Delete() error {
	b.deletes++
	return nil
}

type fakeMenuItem struct {
	id   uint32
	text string
}

type fakeTrayMenuAPI struct {
	items     []fakeMenuItem
	command   uint32
	appendErr error
	destroyed int
}

func (*fakeTrayMenuAPI) Create() (uintptr, error) { return 7, nil }

func (a *fakeTrayMenuAPI) Append(_ uintptr, id uint32, text string) error {
	if a.appendErr != nil {
		return a.appendErr
	}
	a.items = append(a.items, fakeMenuItem{id, text})
	return nil
}

func (a *fakeTrayMenuAPI) Track(_, _ uintptr) (uint32, error) { return a.command, nil }

func (a *fakeTrayMenuAPI) Destroy(uintptr) error {
	a.destroyed++
	return nil
}
