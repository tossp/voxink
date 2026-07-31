package volcengine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/tossp/voxink/internal/asr"
)

const defaultEndpoint = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async"

// AuthMode selects one mutually exclusive Volcengine handshake credential set.
type AuthMode string

const (
	// AuthAPIKey uses the current X-Api-Key and resource ID path.
	AuthAPIKey AuthMode = "api-key"
	// AuthLegacy uses the legacy app key and access key path.
	AuthLegacy AuthMode = "legacy"
)

// AuthConfig contains Volcengine handshake credentials without logging them.
type AuthConfig struct {
	Mode       AuthMode
	APIKey     string
	AppKey     string
	AccessKey  string
	ResourceID string
	RequestID  string
	ConnectID  string
}

// Config configures one Volcengine V3 live recognizer.
type Config struct {
	Endpoint   string
	HTTPClient *http.Client
	Auth       AuthConfig
	Request    RequestConfig
	ReadLimit  int64
}

// Client opens one WebSocket connection for each live session.
type Client struct {
	config Config
}

// NewClient validates the explicit endpoint, authentication, and read limit.
func NewClient(config Config) (*Client, error) {
	if config.Endpoint == "" {
		config.Endpoint = defaultEndpoint
	}
	if config.ReadLimit <= 0 {
		return nil, fmt.Errorf("Volcengine WebSocket read limit must be positive")
	}
	if err := validateAuth(config.Auth); err != nil {
		return nil, err
	}
	return &Client{config: config}, nil
}

// Dial opens one session connection and sends its full client request.
func (c *Client) Dial(ctx context.Context) (asr.LiveSession, error) {
	header := make(http.Header)
	applyAuth(header, c.config.Auth)
	conn, response, err := websocket.Dial(ctx, c.config.Endpoint, &websocket.DialOptions{
		HTTPClient:      c.config.HTTPClient,
		HTTPHeader:      header,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		return nil, fmt.Errorf("dial Volcengine WebSocket (HTTP status %d): %w", status, err)
	}
	conn.SetReadLimit(c.config.ReadLimit)
	session := &Session{conn: conn}
	frame, err := encodeFullRequest(c.config.Request)
	if err != nil {
		_ = conn.CloseNow()
		return nil, err
	}
	if err := session.write(ctx, frame); err != nil {
		_ = conn.CloseNow()
		return nil, fmt.Errorf("send Volcengine full client request: %w", err)
	}
	return session, nil
}

// Session owns one Volcengine WebSocket connection.
type Session struct {
	conn *websocket.Conn

	writeMu  sync.Mutex
	readMu   sync.Mutex
	stateMu  sync.Mutex
	finished bool
	closed   bool
}

// SendPCM sends one PCM chunk as one binary WebSocket message.
func (s *Session) SendPCM(ctx context.Context, pcm []byte) error {
	if len(pcm) == 0 {
		return fmt.Errorf("Volcengine PCM chunk must not be empty")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.canWrite(); err != nil {
		return err
	}
	frame, err := encodeAudio(pcm, false)
	if err != nil {
		return err
	}
	return s.writeLocked(ctx, frame)
}

// FinishInput sends the final empty audio packet exactly once.
func (s *Session) FinishInput(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.canWrite(); err != nil {
		return err
	}
	frame, err := encodeAudio(nil, true)
	if err != nil {
		return err
	}
	if err := s.writeLocked(ctx, frame); err != nil {
		return err
	}
	s.stateMu.Lock()
	s.finished = true
	s.stateMu.Unlock()
	return nil
}

// NextEvent reads and decodes one binary server message.
func (s *Session) NextEvent(ctx context.Context) (asr.LiveEvent, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	if s.isClosed() {
		return asr.LiveEvent{}, errors.New("Volcengine session is closed")
	}
	messageType, payload, err := s.conn.Read(ctx)
	if err != nil {
		s.invalidateOnContext(ctx)
		return asr.LiveEvent{}, fmt.Errorf("read Volcengine WebSocket: %w", err)
	}
	if messageType != websocket.MessageBinary {
		return asr.LiveEvent{}, fmt.Errorf("Volcengine server sent non-binary WebSocket message")
	}
	return decodeServerFrame(payload)
}

// Close performs a normal WebSocket close; it does not imply protocol final.
func (s *Session) Close(ctx context.Context) error {
	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		return nil
	}
	s.closed = true
	s.stateMu.Unlock()

	done := make(chan error, 1)
	go func() {
		done <- s.conn.Close(websocket.StatusNormalClosure, "")
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = s.conn.CloseNow()
		return ctx.Err()
	}
}

func validateAuth(auth AuthConfig) error {
	if auth.ResourceID == "" {
		return fmt.Errorf("Volcengine resource ID must not be empty")
	}
	switch auth.Mode {
	case AuthAPIKey:
		if auth.APIKey == "" {
			return fmt.Errorf("Volcengine API key must not be empty")
		}
		if auth.AppKey != "" || auth.AccessKey != "" {
			return fmt.Errorf("Volcengine API key and legacy credentials are mutually exclusive")
		}
	case AuthLegacy:
		if auth.AppKey == "" || auth.AccessKey == "" {
			return fmt.Errorf("Volcengine legacy app key and access key are both required")
		}
		if auth.APIKey != "" {
			return fmt.Errorf("Volcengine API key and legacy credentials are mutually exclusive")
		}
	default:
		return fmt.Errorf("unsupported Volcengine authentication mode %q", auth.Mode)
	}
	return nil
}

func applyAuth(header http.Header, auth AuthConfig) {
	if auth.Mode == AuthAPIKey {
		header.Set("X-Api-Key", auth.APIKey)
	} else {
		header.Set("X-Api-App-Key", auth.AppKey)
		header.Set("X-Api-Access-Key", auth.AccessKey)
	}
	header.Set("X-Api-Resource-Id", auth.ResourceID)
	if auth.RequestID != "" {
		header.Set("X-Api-Request-Id", auth.RequestID)
	}
	if auth.ConnectID != "" {
		header.Set("X-Api-Connect-Id", auth.ConnectID)
	}
}

func (s *Session) write(ctx context.Context, frame []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.writeLocked(ctx, frame)
}

func (s *Session) writeLocked(ctx context.Context, frame []byte) error {
	if err := s.conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		s.invalidateOnContext(ctx)
		return fmt.Errorf("write Volcengine WebSocket: %w", err)
	}
	return nil
}

func (s *Session) canWrite() error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.closed {
		return errors.New("Volcengine session is closed")
	}
	if s.finished {
		return errors.New("Volcengine session input is finished")
	}
	return nil
}

func (s *Session) isClosed() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.closed
}

func (s *Session) invalidateOnContext(ctx context.Context) {
	if ctx.Err() == nil {
		return
	}
	s.stateMu.Lock()
	s.closed = true
	s.stateMu.Unlock()
	_ = s.conn.CloseNow()
}
