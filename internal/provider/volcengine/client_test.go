package volcengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/tossp/voxink/internal/asr"
)

func TestClientMockSessionLifecycle(t *testing.T) {
	serverErr := make(chan error, 1)
	responses := [][]byte{
		testServerFrame(t, flagPositiveSequence, compressionGZIP, 1, []byte(`{"result":{"text":"你","utterances":[{"text":"你","definite":true}]}}`)),
		testServerFrame(t, flagPositiveSequence|flagLastNoSequence, compressionGZIP, 2, []byte(`{"result":{"text":"你好"}}`)),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-Key"); got != "local-test-key" {
			serverErr <- fmt.Errorf("X-Api-Key = %q", got)
			return
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != "local-resource" {
			serverErr <- fmt.Errorf("X-Api-Resource-Id = %q", got)
			return
		}
		if got := r.Header.Get("X-Api-Request-Id"); got != "local-request-id" {
			serverErr <- fmt.Errorf("X-Api-Request-Id = %q", got)
			return
		}
		if extension := r.Header.Get("Sec-Websocket-Extensions"); extension != "" {
			serverErr <- fmt.Errorf("compression extension was offered: %q", extension)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		want := []struct {
			message byte
			flag    byte
			pcm     []byte
		}{
			{message: messageFullClient, flag: flagNoSequence},
			{message: messageAudioOnly, flag: flagNoSequence, pcm: []byte{1, 2, 3, 4}},
			{message: messageAudioOnly, flag: flagLastNoSequence, pcm: []byte{}},
		}
		for index, expected := range want {
			messageType, frame, err := conn.Read(ctx)
			if err != nil {
				serverErr <- fmt.Errorf("read message %d: %w", index, err)
				return
			}
			if messageType != websocket.MessageBinary {
				serverErr <- fmt.Errorf("message %d type = %v", index, messageType)
				return
			}
			if frame[1]>>4 != expected.message || frame[1]&0x0f != expected.flag {
				serverErr <- fmt.Errorf("message %d header = %x", index, frame[:4])
				return
			}
			if expected.message == messageAudioOnly {
				pcm, err := sizedPayload(frame[4:], compressionGZIP)
				if err != nil {
					serverErr <- fmt.Errorf("decode message %d PCM: %w", index, err)
					return
				}
				if !bytes.Equal(pcm, expected.pcm) {
					serverErr <- fmt.Errorf("message %d PCM = %x", index, pcm)
					return
				}
			}
		}
		for _, response := range responses {
			if err := conn.Write(ctx, websocket.MessageBinary, response); err != nil {
				serverErr <- err
				return
			}
		}
		serverErr <- nil
	}))
	defer server.Close()

	client := newTestClient(t, websocketURL(server.URL), 64*1024, AuthConfig{
		Mode: AuthAPIKey, APIKey: "local-test-key", ResourceID: "local-resource", RequestID: "local-request-id",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.Dial(ctx)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	if err := session.SendPCM(ctx, []byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("SendPCM() error = %v", err)
	}
	if err := session.FinishInput(ctx); err != nil {
		t.Fatalf("FinishInput() error = %v", err)
	}
	if err := session.SendPCM(ctx, []byte{5, 6}); err == nil {
		t.Fatal("SendPCM() after FinishInput error = nil")
	}
	first, err := session.NextEvent(ctx)
	if err != nil {
		t.Fatalf("NextEvent(first) error = %v", err)
	}
	if first.Text != "你" || first.ProtocolTerminal || len(first.Utterances) != 1 || !first.Utterances[0].Stable {
		t.Fatalf("first event = %+v", first)
	}
	second, err := session.NextEvent(ctx)
	if err != nil {
		t.Fatalf("NextEvent(second) error = %v", err)
	}
	if second.Text != "你好" || !second.ProtocolTerminal || second.Sequence != 2 {
		t.Fatalf("second event = %+v", second)
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
	defer closeCancel()
	_ = session.Close(closeCtx)
	if err := <-serverErr; err != nil {
		t.Fatalf("mock server error = %v", err)
	}
}

func TestClientLegacyAuthentication(t *testing.T) {
	seen := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Clone()
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _, _ = conn.Read(ctx)
	}))
	defer server.Close()

	client := newTestClient(t, websocketURL(server.URL), 1024, AuthConfig{
		Mode: AuthLegacy, AppKey: "local-app", AccessKey: "local-access", ResourceID: "local-resource",
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	session, err := client.Dial(ctx)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	_ = session.Close(ctx)
	header := <-seen
	if header.Get("X-Api-App-Key") != "local-app" || header.Get("X-Api-Access-Key") != "local-access" {
		t.Fatalf("legacy headers missing")
	}
	if header.Get("X-Api-Key") != "" {
		t.Fatalf("legacy request included X-Api-Key")
	}
}

func TestClientContextCancellationInvalidatesSession(t *testing.T) {
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_, _, _ = conn.Read(context.Background())
		close(ready)
		_, _, _ = conn.Read(context.Background())
	}))
	defer server.Close()

	client := newTestClient(t, websocketURL(server.URL), 1024, AuthConfig{
		Mode: AuthAPIKey, APIKey: "local-key", ResourceID: "local-resource",
	})
	session, err := client.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	<-ready
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = session.NextEvent(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("NextEvent() error = %v, want deadline", err)
	}
	if err := session.SendPCM(context.Background(), []byte{1, 2}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("SendPCM() after cancellation error = %v", err)
	}
}

func TestClientReadLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _, _ = conn.Read(ctx)
		_ = conn.Write(ctx, websocket.MessageBinary, make([]byte, 128))
	}))
	defer server.Close()

	client := newTestClient(t, websocketURL(server.URL), 64, AuthConfig{
		Mode: AuthAPIKey, APIKey: "local-key", ResourceID: "local-resource",
	})
	session, err := client.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	_, err = session.NextEvent(context.Background())
	if err == nil || !strings.Contains(err.Error(), "message too big") {
		t.Fatalf("NextEvent() error = %v, want read limit failure", err)
	}
}

func TestNewClientValidatesAuthenticationAndReadLimit(t *testing.T) {
	tests := []Config{
		{ReadLimit: 0, Auth: AuthConfig{Mode: AuthAPIKey, APIKey: "x", ResourceID: "r"}},
		{ReadLimit: 1, Auth: AuthConfig{Mode: AuthAPIKey, ResourceID: "r"}},
		{ReadLimit: 1, Auth: AuthConfig{Mode: AuthAPIKey, APIKey: "x", AppKey: "y", ResourceID: "r"}},
		{ReadLimit: 1, Auth: AuthConfig{Mode: AuthLegacy, AppKey: "x", ResourceID: "r"}},
		{ReadLimit: 1, Auth: AuthConfig{Mode: AuthLegacy, AppKey: "x", AccessKey: "y"}},
	}
	for index, config := range tests {
		if _, err := NewClient(config); err == nil {
			t.Fatalf("case %d: NewClient() error = nil", index)
		}
	}
}

func newTestClient(t *testing.T, endpoint string, readLimit int64, auth AuthConfig) *Client {
	t.Helper()
	client, err := NewClient(Config{
		Endpoint: endpoint, ReadLimit: readLimit, Auth: auth,
		Request: RequestConfig{UserID: "local-user", EnableITN: true, EnablePunc: true},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

func websocketURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

var _ asr.LiveRecognizer = (*Client)(nil)
var _ asr.LiveSession = (*Session)(nil)
