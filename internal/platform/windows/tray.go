package windows

import "errors"

const (
	trayIconID       = 1
	trayMenuToggleID = 1001
	trayMenuExitID   = 1002

	trayLeftDoubleClick = 0x0203
	trayRightButtonUp   = 0x0205
)

type trayAction uint8

const (
	trayActionNone trayAction = iota
	trayActionToggle
	trayActionExit
	trayActionMenu
)

type trayBackend interface {
	Add(hwnd uintptr, tooltip string) error
	Update(tooltip string) error
	Popup(hwnd uintptr, toggleText string) (uint32, error)
	Delete() error
}

type trayRuntime struct {
	backend  trayBackend
	hwnd     uintptr
	status   ViewStatus
	added    bool
	closed   bool
	closeErr error
}

func newTrayRuntime(backend trayBackend, hwnd uintptr) *trayRuntime {
	return &trayRuntime{backend: backend, hwnd: hwnd, status: ViewIdle}
}

func (t *trayRuntime) Open() error {
	if t.added {
		return nil
	}
	if t.closed {
		return ErrOverlayClosed
	}
	if err := t.backend.Add(t.hwnd, trayTooltip(t.status)); err != nil {
		return err
	}
	t.added = true
	return nil
}

func (t *trayRuntime) Update(view View) error {
	status := trayViewStatus(view.Status)
	if status == t.status {
		return nil
	}
	t.status = status
	if !t.added || t.closed {
		return nil
	}
	return t.backend.Update(trayTooltip(t.status))
}

func (t *trayRuntime) HandleNotification(message uint32) (trayAction, error) {
	action := trayNotificationAction(message)
	if action != trayActionMenu {
		return action, nil
	}
	command, err := t.backend.Popup(t.hwnd, trayToggleText(t.status))
	if err != nil {
		return trayActionNone, err
	}
	return trayCommandAction(command), nil
}

func (t *trayRuntime) Close() error {
	if t.closed {
		return t.closeErr
	}
	t.closed = true
	if t.added {
		t.closeErr = t.backend.Delete()
		t.added = false
	}
	return t.closeErr
}

func trayNotificationAction(message uint32) trayAction {
	switch message {
	case trayLeftDoubleClick:
		return trayActionToggle
	case trayRightButtonUp:
		return trayActionMenu
	default:
		return trayActionNone
	}
}

func trayCommandAction(command uint32) trayAction {
	switch command {
	case trayMenuToggleID:
		return trayActionToggle
	case trayMenuExitID:
		return trayActionExit
	default:
		return trayActionNone
	}
}

func trayViewStatus(status ViewStatus) ViewStatus {
	if status == ViewListening || status == ViewTranscribing {
		return status
	}
	return ViewIdle
}

func trayTooltip(status ViewStatus) string {
	switch status {
	case ViewListening:
		return "VoxInk - Capturing"
	case ViewTranscribing:
		return "VoxInk - Transcribing"
	default:
		return "VoxInk - Idle"
	}
}

func trayToggleText(status ViewStatus) string {
	if status == ViewListening {
		return "停止"
	}
	return "开始"
}

type trayMenuAPI interface {
	Create() (uintptr, error)
	Append(menu uintptr, id uint32, text string) error
	Track(menu, hwnd uintptr) (uint32, error)
	Destroy(menu uintptr) error
}

func popupTrayMenu(api trayMenuAPI, hwnd uintptr, toggleText string) (command uint32, err error) {
	menu, err := api.Create()
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, api.Destroy(menu)) }()
	if err := api.Append(menu, trayMenuToggleID, toggleText); err != nil {
		return 0, err
	}
	if err := api.Append(menu, trayMenuExitID, "退出"); err != nil {
		return 0, err
	}
	return api.Track(menu, hwnd)
}
