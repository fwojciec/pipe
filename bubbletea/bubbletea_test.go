package bubbletea_test

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/require"
)

// initModel creates a model and sends a WindowSizeMsg to initialize the viewport.
func initModel(t *testing.T, run bt.AgentFunc) bt.Model {
	t.Helper()
	session := &pipe.Session{}
	theme := pipe.DefaultTheme()
	m := bt.New(run, session, theme)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, ok := updated.(bt.Model)
	require.True(t, ok)
	return model
}

// initModelWithSize creates a model with a custom terminal size.
func initModelWithSize(t *testing.T, run bt.AgentFunc, width, height int) bt.Model {
	t.Helper()
	session := &pipe.Session{}
	theme := pipe.DefaultTheme()
	m := bt.New(run, session, theme)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	model, ok := updated.(bt.Model)
	require.True(t, ok)
	return model
}

// updateModel sends a message and returns the updated Model.
func updateModel(t *testing.T, m bt.Model, msg tea.Msg) bt.Model {
	t.Helper()
	updated, _ := m.Update(msg)
	model, ok := updated.(bt.Model)
	require.True(t, ok)
	return model
}

// nopAgent is a mock agent that does nothing.
func nopAgent(_ context.Context, _ *pipe.Session, _ func(pipe.Event)) error {
	return nil
}
