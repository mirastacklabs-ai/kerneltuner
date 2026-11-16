package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// maxStateBytes caps what ReadState accepts: the file is written by the agent
// but may sit in a directory other users can influence.
const maxStateBytes = 4 << 20

// RuleState is one tracked rule in the state file.
type RuleState struct {
	ID           string    `json:"id"`
	Target       string    `json:"target"`
	State        string    `json:"state"`
	Since        time.Time `json:"since"`
	LastEvidence string    `json:"last_evidence"`
	Current      string    `json:"current"`
	Proposed     string    `json:"proposed"`
}

// StateFile is the JSON document the agent maintains for `kerneltuner status`.
type StateFile struct {
	Updated      time.Time   `json:"updated"`
	Interval     string      `json:"interval"`
	Host         string      `json:"host"`
	Rules        []RuleState `json:"rules"`
	RecentEvents []Event     `json:"recent_events"`
}

// WriteState writes sf atomically: a temp file in the same directory, then rename.
// The directory is created with mode 0755 if missing.
func WriteState(path string, sf *StateFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// ReadState loads a state file.
func ReadState(path string) (*StateFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxStateBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxStateBytes {
		return nil, fmt.Errorf("state file %s is larger than %d bytes", path, maxStateBytes)
	}
	var sf StateFile
	if err := json.Unmarshal(b, &sf); err != nil {
		return nil, err
	}
	return &sf, nil
}
