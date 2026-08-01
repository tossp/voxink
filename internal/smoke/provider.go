package smoke

import (
	"context"
	"errors"
	"time"

	"github.com/tossp/voxink/internal/app"
	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/credential"
	"github.com/tossp/voxink/internal/provider/mimo"
	"github.com/tossp/voxink/internal/provider/volcengine"
)

// volcFrameBytes is 100 ms * 16,000 samples/s * 1 mono channel * 2 bytes/sample (PCM16).
const volcFrameBytes = 100 * 16_000 * 2 / 1000

func newProviderRunner(provider Provider, getenv func(string) string, store credential.Store) (providerRunner, error) {
	switch provider {
	case ProviderVolc:
		config, err := app.LoadVolcengineConfigWithCredentials(getenv, store)
		if err != nil {
			return nil, err
		}
		client, err := volcengine.NewClient(config)
		if err != nil {
			return nil, err
		}
		return volcRunner{client: client}, nil
	case ProviderMiMo:
		config, err := app.LoadMiMoConfigWithCredentials(getenv, store)
		if err != nil {
			return nil, err
		}
		transcriber, err := mimo.NewTranscriber(config)
		if err != nil {
			return nil, err
		}
		return mimoRunner{transcriber: transcriber}, nil
	default:
		return nil, errors.New("unsupported Provider smoke target")
	}
}

type volcRunner struct{ client asr.LiveRecognizer }

func (r volcRunner) Run(ctx context.Context, input *audioInput) (result providerResult, runErr error) {
	session, err := r.client.Dial(ctx)
	if err != nil {
		return result, err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := session.Close(closeCtx); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()

	for offset := 0; offset < len(input.pcm); offset += volcFrameBytes {
		end := min(offset+volcFrameBytes, len(input.pcm))
		if err := session.SendPCM(ctx, input.pcm[offset:end]); err != nil {
			return result, err
		}
	}
	if err := session.FinishInput(ctx); err != nil {
		return result, err
	}
	for {
		event, err := session.NextEvent(ctx)
		if err != nil {
			return result, err
		}
		result.eventCount++
		if event.ProtocolTerminal {
			result.finalReceived = true
			result.protocolTerminal = true
			return result, nil
		}
	}
}

type wavTranscriber interface {
	TranscribeWAV(context.Context, []byte) (string, error)
}

type mimoRunner struct{ transcriber wavTranscriber }

func (r mimoRunner) Run(ctx context.Context, input *audioInput) (providerResult, error) {
	_, err := r.transcriber.TranscribeWAV(ctx, input.wav)
	if err != nil {
		return providerResult{}, err
	}
	return providerResult{finalReceived: true}, nil
}

func classifyProviderError(ctx context.Context, err error) Code {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return CodeTimeout
	}
	var mimoHTTP *mimo.HTTPError
	if errors.As(err, &mimoHTTP) {
		return classifyHTTPStatus(mimoHTTP.StatusCode)
	}
	var volcHandshake *volcengine.HandshakeError
	if errors.As(err, &volcHandshake) {
		return classifyHTTPStatus(volcHandshake.StatusCode)
	}
	var responseError *mimo.ResponseError
	if errors.As(err, &responseError) {
		return CodeResponseInvalid
	}
	var transportError *mimo.TransportError
	if errors.As(err, &transportError) {
		return CodeProviderUnavailable
	}
	var serverError *volcengine.ServerError
	if errors.As(err, &serverError) || errors.Is(err, volcengine.ErrProtocol) {
		return CodeProtocolFailed
	}
	return CodeInternalFailure
}

func classifyHTTPStatus(status int) Code {
	switch {
	case status == 401 || status == 403:
		return CodeUnauthorized
	case status == 429:
		return CodeRateLimited
	case status == 0 || status >= 500:
		return CodeProviderUnavailable
	default:
		return CodeProtocolFailed
	}
}

func statusClass(status int) HTTPStatusClass {
	switch {
	case status >= 400 && status < 500:
		return HTTPStatus4xx
	case status >= 500 && status < 600:
		return HTTPStatus5xx
	default:
		return ""
	}
}
