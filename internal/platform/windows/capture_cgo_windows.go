//go:build windows && cgo

package windows

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/gen2brain/malgo"
)

const (
	captureSampleRate = 16_000
	captureChannels   = 1
)

// NewCapture initializes a fixed-format malgo WASAPI capture device on its owner thread.
func NewCapture() (*Capture, error) {
	capture := newCapture()
	ready := make(chan error, 1)
	go capture.runOwner(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return capture, nil
}

func (c *Capture) runOwner(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	context, err := malgo.InitContext([]malgo.Backend{malgo.BackendWasapi}, malgo.ContextConfig{}, nil)
	if err != nil {
		ready <- fmt.Errorf("initialize WASAPI context: %w", err)
		return
	}

	config := malgo.DefaultDeviceConfig(malgo.Capture)
	config.Capture.Format = malgo.FormatS16
	config.Capture.Channels = captureChannels
	config.SampleRate = captureSampleRate
	callbacks := malgo.DeviceCallbacks{
		Data: func(_ []byte, inputSamples []byte, _ uint32) {
			c.ingress.accept(inputSamples)
		},
	}
	device, err := malgo.InitDevice(context.Context, config, callbacks)
	if err != nil {
		_ = context.Uninit()
		context.Free()
		ready <- fmt.Errorf("initialize WASAPI capture device: %w", err)
		return
	}

	if err := c.validateAndRecordFormat(device); err != nil {
		device.Uninit()
		_ = context.Uninit()
		context.Free()
		ready <- err
		return
	}
	ready <- nil

	state := captureStopped
	for command := range c.commands {
		next, operate, transitionErr := transitionCapture(state, command.action)
		if transitionErr != nil || !operate {
			command.reply <- transitionErr
			continue
		}

		var operationErr error
		switch command.action {
		case actionStart:
			operationErr = device.Start()
		case actionStop:
			operationErr = device.Stop()
		case actionClose:
			operationErr = closeMalgo(device, context)
			state = captureClosed
			command.reply <- operationErr
			return
		}
		if operationErr == nil {
			state = next
		}
		command.reply <- operationErr
	}
}

func (c *Capture) validateAndRecordFormat(device *malgo.Device) error {
	format := AudioFormat{
		CallbackFormat:     formatName(device.CaptureFormat()),
		CallbackChannels:   device.CaptureChannels(),
		CallbackSampleRate: device.SampleRate(),
		InternalFormat:     formatName(device.CaptureInternalFormat()),
		InternalChannels:   device.CaptureInternalChannels(),
		InternalSampleRate: device.CaptureInternalSampleRate(),
	}
	c.format.Store(&format)
	if device.CaptureFormat() != malgo.FormatS16 ||
		device.CaptureChannels() != captureChannels ||
		device.SampleRate() != captureSampleRate {
		return fmt.Errorf("unexpected callback format: %+v", format)
	}
	return nil
}

func closeMalgo(device *malgo.Device, context *malgo.AllocatedContext) error {
	var closeErr error
	closeErr = errors.Join(closeErr, device.Stop())
	device.Uninit()
	closeErr = errors.Join(closeErr, context.Uninit())
	context.Free()
	return closeErr
}

func formatName(format malgo.FormatType) string {
	if format == malgo.FormatS16 {
		return "s16"
	}
	return fmt.Sprintf("malgo-format-%d", format)
}
