// Package transcription is a compatibility facade for transcriptionstate.
// The durable state implementation lives in transcriptionstate so callers can
// use either the descriptive package name or the existing module seam.
package transcription

import (
	"github.com/rob137/risper/config"
	"github.com/rob137/risper/session"
	state "github.com/rob137/risper/transcriptionstate"
)

type State = state.State

func Current(cfg config.Config) (*State, error) { return state.Current(cfg) }
func Start(cfg config.Config, metadata *session.Metadata, profileID string) error {
	return state.Start(cfg, metadata, profileID)
}
func SetWorkerPID(cfg config.Config, workerPID int) error {
	return state.SetWorkerPID(cfg, workerPID)
}
func Finish(cfg config.Config) error { return state.Finish(cfg) }
func Cancel(cfg config.Config, current *State) (bool, error) {
	if current == nil {
		return false, nil
	}
	if err := state.Cancel(cfg, current); err != nil {
		return false, err
	}
	return true, nil
}
