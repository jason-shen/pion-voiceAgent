package tts

import "fmt"

type ErrMissingAPIKey struct {
	Provider string
	EnvVar   string
}

func (e ErrMissingAPIKey) Error() string {
	return fmt.Sprintf("TTS provider %q requires %s to be set", e.Provider, e.EnvVar)
}
