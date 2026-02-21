package bubbletea_test

import (
	"errors"
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestErrorBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("renders error prefix and message", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewErrorBlock(errors.New("something broke"), styles)
		view := block.View(80)
		assert.Contains(t, view, "Error")
		assert.Contains(t, view, "something broke")
	})
}
