//go:build (windows && cgo) || fyne_gui

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	runtimeapp "github.com/tossp/voxink/internal/app"
	"github.com/tossp/voxink/internal/credential"
	"github.com/tossp/voxink/internal/history"
)

const (
	applicationID  = "io.github.tossp.voxink"
	saveFailedText = "保存失败，请检查输入或存储状态。"
	saveOKText     = "设置已保存。"
)

type desktopWidgets struct {
	native         fyne.App
	mainWindow     fyne.Window
	settingsWindow fyne.Window
	status         *widget.Label
	toggle         *widget.Button
	history        *widget.List
	credential     map[credential.Name]*widget.Entry
	configured     map[credential.Name]*widget.Label
	settingsStatus *widget.Label
	hotkey         *hotkeyEntry
	volcEndpoint   *widget.Entry
	volcReadLimit  *widget.Entry
	mimoEndpoint   *widget.Entry
	mimoAuthMode   *widget.Select
}

// Run creates both windows and runs Fyne on the calling goroutine.
func (a *App) Run() {
	native := fyneapp.NewWithID(applicationID)
	a.widgets.native = native
	a.buildMainWindow()
	a.buildSettingsWindow()
	done := make(chan struct{})
	defer close(done)
	go a.consumeEvents(done)
	a.widgets.mainWindow.Show()
	native.Run()
}

func (a *App) consumeEvents(done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case event, ok := <-a.options.Events:
			if !ok {
				return
			}
			fyne.Do(func() {
				a.HandleEvent(event)
				a.refreshMain()
				switch event.Action {
				case runtimeapp.ActionOpenMain:
					a.showMain()
				case runtimeapp.ActionOpenSettings:
					a.showSettings()
				case runtimeapp.ActionQuit:
					a.widgets.native.Quit()
				}
			})
		}
	}
}

func (a *App) buildMainWindow() {
	window := a.widgets.native.NewWindow("VoxInk")
	window.Resize(fyne.NewSize(760, 520))
	window.CenterOnScreen()
	window.SetCloseIntercept(window.Hide)

	status := widget.NewLabel("")
	status.TextStyle = fyne.TextStyle{Bold: true}
	toggle := widget.NewButtonWithIcon("", theme.MediaRecordIcon(), a.Toggle)
	settingsButton := widget.NewButtonWithIcon("设置", theme.SettingsIcon(), a.showSettings)
	list := widget.NewList(
		func() int { return len(a.History()) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapWord
			return label
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			entries := a.History()
			if id >= len(entries) {
				return
			}
			object.(*widget.Label).SetText(historyText(entries[id]))
		},
	)
	a.widgets.mainWindow = window
	a.widgets.status = status
	a.widgets.toggle = toggle
	a.widgets.history = list
	window.SetContent(container.NewBorder(
		container.NewVBox(status, container.NewHBox(toggle, settingsButton), widget.NewSeparator()),
		nil, nil, nil, list,
	))
	a.refreshMain()
}

func historyText(entry history.Entry) string {
	return fmt.Sprintf("%s  %s  %s\n%s", entry.Time.Local().Format("2006-01-02 15:04:05"), entry.Provider, entry.Mode, entry.Final)
}

func (a *App) refreshMain() {
	if a.widgets.status == nil {
		return
	}
	status := a.Status()
	a.widgets.status.SetText("状态: " + string(status))
	if status == runtimeapp.StatusCapturing {
		a.widgets.toggle.SetText("停止")
		a.widgets.toggle.SetIcon(theme.MediaStopIcon())
	} else {
		a.widgets.toggle.SetText("开始")
		a.widgets.toggle.SetIcon(theme.MediaRecordIcon())
	}
	a.widgets.history.Refresh()
}

func (a *App) buildSettingsWindow() {
	window := a.widgets.native.NewWindow("VoxInk 设置")
	window.Resize(fyne.NewSize(700, 660))
	window.CenterOnScreen()
	window.SetCloseIntercept(window.Hide)
	a.widgets.settingsWindow = window
	a.widgets.credential = make(map[credential.Name]*widget.Entry, len(credential.Names()))
	a.widgets.configured = make(map[credential.Name]*widget.Label, len(credential.Names()))

	form := a.Form()
	items := make([]*widget.FormItem, 0, 11)
	for _, name := range credential.Names() {
		entry := widget.NewPasswordEntry()
		entry.PlaceHolder = "留空不修改"
		status := widget.NewLabel(configuredText(a.CredentialConfigured(name)))
		a.widgets.credential[name] = entry
		a.widgets.configured[name] = status
		items = append(items, widget.NewFormItem(string(name), container.NewBorder(nil, nil, nil, status, entry)))
	}

	var captureButton *widget.Button
	hotkey := newHotkeyEntry(a.currentModifiers, func(focused bool) {
		if captureButton == nil {
			return
		}
		if focused && a.options.CaptureSupported {
			captureButton.Enable()
		} else {
			captureButton.Disable()
		}
	})
	hotkey.SetText(form.Hotkey)
	captureButton = widget.NewButton("捕获", func() {
		window.Canvas().Focus(hotkey)
		if window.Canvas().Focused() == hotkey && a.options.CaptureSupported {
			hotkey.capturing = true
		}
	})
	captureButton.Disable()
	a.widgets.hotkey = hotkey
	items = append(items, widget.NewFormItem("hotkey", container.NewBorder(nil, nil, nil, captureButton, hotkey)))

	a.widgets.volcEndpoint = widget.NewEntry()
	a.widgets.volcEndpoint.SetText(form.VolcEndpoint)
	a.widgets.volcReadLimit = widget.NewEntry()
	a.widgets.volcReadLimit.SetText(form.VolcReadLimitBytes)
	a.widgets.mimoEndpoint = widget.NewEntry()
	a.widgets.mimoEndpoint.SetText(form.MiMoEndpoint)
	a.widgets.mimoAuthMode = widget.NewSelect([]string{"api-key", "bearer"}, nil)
	a.widgets.mimoAuthMode.SetSelected(form.MiMoAuthMode)
	items = append(items,
		widget.NewFormItem("volc-endpoint", a.widgets.volcEndpoint),
		widget.NewFormItem("volc-read-limit-bytes", a.widgets.volcReadLimit),
		widget.NewFormItem("mimo-endpoint", a.widgets.mimoEndpoint),
		widget.NewFormItem("mimo-auth-mode", a.widgets.mimoAuthMode),
	)
	status := widget.NewLabel("")
	a.widgets.settingsStatus = status
	save := widget.NewButtonWithIcon("保存", theme.DocumentSaveIcon(), a.saveSettings)
	closeButton := widget.NewButtonWithIcon("关闭", theme.WindowCloseIcon(), window.Hide)
	content := container.NewBorder(nil, container.NewVBox(widget.NewSeparator(), status, container.NewHBox(save, closeButton)), nil, nil, widget.NewForm(items...))
	window.SetContent(container.NewVScroll(content))
}

func (a *App) currentModifiers() fyne.KeyModifier {
	driver, ok := a.widgets.native.Driver().(desktop.Driver)
	if !ok {
		return 0
	}
	return driver.CurrentKeyModifiers()
}

func (a *App) saveSettings() {
	values := FormValues{
		Hotkey: a.widgets.hotkey.Text, VolcEndpoint: a.widgets.volcEndpoint.Text,
		VolcReadLimitBytes: a.widgets.volcReadLimit.Text, MiMoEndpoint: a.widgets.mimoEndpoint.Text,
		MiMoAuthMode: a.widgets.mimoAuthMode.Selected, Credentials: make(map[credential.Name]string),
	}
	for name, entry := range a.widgets.credential {
		values.Credentials[name] = entry.Text
	}
	if err := a.Save(values); err != nil {
		a.widgets.settingsStatus.SetText(saveFailedText)
		return
	}
	for name, entry := range a.widgets.credential {
		entry.SetText("")
		a.widgets.configured[name].SetText(configuredText(a.CredentialConfigured(name)))
	}
	a.widgets.settingsStatus.SetText(saveOKText)
}

func configuredText(configured bool) string {
	if configured {
		return "configured"
	}
	return "not configured"
}

func (a *App) showMain() {
	a.widgets.mainWindow.Show()
	a.widgets.mainWindow.RequestFocus()
}

func (a *App) showSettings() {
	a.widgets.settingsWindow.Show()
	a.widgets.settingsWindow.RequestFocus()
}
