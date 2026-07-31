package asr

import "context"

// LiveRecognizer opens one true-streaming recognition connection per session.
type LiveRecognizer interface {
	Dial(context.Context) (LiveSession, error)
}

// LiveSession owns one live provider connection and its input lifecycle.
type LiveSession interface {
	SendPCM(context.Context, []byte) error
	FinishInput(context.Context) error
	NextEvent(context.Context) (LiveEvent, error)
	Close(context.Context) error
}

// LiveEvent is one provider protocol update, not a VoxInk session Final.
type LiveEvent struct {
	Text             string
	Utterances       []LiveUtterance
	Sequence         int32
	HasSequence      bool
	ProtocolTerminal bool
}

// LiveUtterance describes one provider utterance and its stability.
type LiveUtterance struct {
	Text    string
	StartMS int
	EndMS   int
	Stable  bool
}
