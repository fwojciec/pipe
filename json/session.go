package json

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fwojciec/pipe"
)

// envelope is the v1 wire format for a persisted session.
type envelope struct {
	Version      int          `json:"version"`
	ID           string       `json:"id"`
	SystemPrompt string       `json:"system_prompt"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	Messages     []messageDTO `json:"messages"`
}

// MarshalSession serializes a Session to JSON in v1 envelope format.
func MarshalSession(s pipe.Session) ([]byte, error) {
	env := envelope{
		Version:      1,
		ID:           s.ID,
		SystemPrompt: s.SystemPrompt,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
		Messages:     make([]messageDTO, len(s.Messages)),
	}
	for i, msg := range s.Messages {
		dto, err := marshalMessage(msg)
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", i, err)
		}
		env.Messages[i] = dto
	}
	return json.MarshalIndent(env, "", "  ")
}

// UnmarshalSession deserializes a Session from JSON in v1 envelope format.
func UnmarshalSession(data []byte) (pipe.Session, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return pipe.Session{}, fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.Version != 1 {
		return pipe.Session{}, fmt.Errorf("unsupported envelope version: %d", env.Version)
	}
	msgs := make([]pipe.Message, len(env.Messages))
	for i, dto := range env.Messages {
		msg, err := unmarshalMessage(dto)
		if err != nil {
			return pipe.Session{}, fmt.Errorf("message %d: %w", i, err)
		}
		msgs[i] = msg
	}
	return pipe.Session{
		ID:           env.ID,
		SystemPrompt: env.SystemPrompt,
		CreatedAt:    env.CreatedAt,
		UpdatedAt:    env.UpdatedAt,
		Messages:     msgs,
	}, nil
}

// Save writes a Session to a JSON file, creating parent directories as needed.
func Save(path string, s pipe.Session) error {
	data, err := MarshalSession(s)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// Load reads a Session from a JSON file.
func Load(path string) (pipe.Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pipe.Session{}, fmt.Errorf("read file: %w", err)
	}
	return UnmarshalSession(data)
}
