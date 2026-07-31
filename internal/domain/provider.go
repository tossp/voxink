package domain

// ProviderKind identifies a configured recognition backend.
type ProviderKind string

const (
	// ProviderVolcengineV3 is the Volcengine V3 streaming ASR backend.
	ProviderVolcengineV3 ProviderKind = "volcengine-v3"
	// ProviderMiMoASR is the MiMo mimo-v2.5-asr batch backend.
	ProviderMiMoASR ProviderKind = "mimo-v2.5-asr"
	// ProviderMiMoV25 is the MiMo mimo-v2.5 audio-understanding backend.
	// It may also be evaluated as a prompt-configured completed-audio transcriber.
	ProviderMiMoV25 ProviderKind = "mimo-v2.5"
	// ProviderMOSI is the MOSI moss-transcribe batch backend.
	ProviderMOSI ProviderKind = "mosi-moss-transcribe"
)

// RecognitionMode separates microphone streaming, completed-audio ASR, and
// future audio understanding so they are not forced into one interface.
type RecognitionMode string

const (
	// RecognitionStreaming sends audio while capture is in progress.
	RecognitionStreaming RecognitionMode = "streaming"
	// RecognitionBatch submits complete audio for transcription.
	RecognitionBatch RecognitionMode = "batch"
	// RecognitionAudioUnderstanding is for semantic analysis of complete audio.
	// A provider can separately declare a batch transcription capability.
	RecognitionAudioUnderstanding RecognitionMode = "audio-understanding"
)

// ProviderCapability records stable routing facts without defining a network API.
type ProviderCapability struct {
	Kind             ProviderKind
	RecognitionMode  RecognitionMode
	AcceptsLiveAudio bool
	StreamsResults   bool
}
