package selfcheck

import (
	"time"

	platformwindows "github.com/tossp/voxink/internal/platform/windows"
)

type windowsCapture struct{ capture *platformwindows.Capture }

func (c windowsCapture) Format() (audioFormat, bool) {
	format, ok := c.capture.Format()
	metricFormat := metricUnknown
	if format.CallbackFormat == "s16" {
		metricFormat = MetricPCM16LE
	}
	return audioFormat{Format: metricFormat, Channels: format.CallbackChannels, SampleRate: format.CallbackSampleRate}, ok
}
func (c windowsCapture) Start() error           { return c.capture.Start() }
func (c windowsCapture) Stop() error            { return c.capture.Stop() }
func (c windowsCapture) Close() error           { return c.capture.Close() }
func (c windowsCapture) PCM() <-chan []byte     { return c.capture.PCM() }
func (c windowsCapture) Levels() <-chan float64 { return c.capture.Levels() }
func (c windowsCapture) Errors() <-chan error   { return c.capture.Errors() }
func (c windowsCapture) OverflowCount() uint64  { return c.capture.OverflowCount() }

type windowsInteractive struct{ overlay *platformwindows.Overlay }

func (o windowsInteractive) Run() error               { return o.overlay.Run() }
func (o windowsInteractive) Close() error             { return o.overlay.Close() }
func (o windowsInteractive) Toggles() <-chan struct{} { return o.overlay.Toggles() }
func (o windowsInteractive) ShowNotice(notice string) {
	o.overlay.Update(platformwindows.View{Status: platformwindows.ViewListening, Partial: "Self-check", Notice: notice})
}

func defaultDependencies() dependencies {
	return dependencies{
		now: timeNow,
		newAudio: func() (audioCapture, error) {
			capture, err := platformwindows.NewCapture()
			if err != nil {
				return nil, err
			}
			return windowsCapture{capture}, nil
		},
		newInteractive: func() interactiveOverlay {
			return windowsInteractive{platformwindows.NewOverlay()}
		},
	}
}

var timeNow = func() time.Time { return time.Now() }
