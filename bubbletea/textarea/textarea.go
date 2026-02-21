// Package textarea provides a minimal multi-line text input for Bubble Tea.
// Forked from charmbracelet/bubbles textarea, stripped of line numbers,
// prompt rendering, placeholder animation, and the Styles system.
// Fixes cache invalidation in SetWidth and adds CheckInputComplete
// callback with auto-grow.
package textarea

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/runeutil"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	rw "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

const (
	minHeight        = 1
	defaultWidth     = 40
	defaultMaxHeight = 99
	maxLines         = 10000
)

// InputHeightMsg is emitted when the textarea's visible height changes
// due to auto-grow.
type InputHeightMsg struct {
	Height int
}

// KeyMap is the key bindings for the textarea.
type KeyMap struct {
	CharacterBackward          key.Binding
	CharacterForward           key.Binding
	DeleteAfterCursor          key.Binding
	DeleteBeforeCursor         key.Binding
	DeleteCharacterBackward    key.Binding
	DeleteCharacterForward     key.Binding
	DeleteWordBackward         key.Binding
	DeleteWordForward          key.Binding
	InsertNewline              key.Binding
	LineEnd                    key.Binding
	LineNext                   key.Binding
	LinePrevious               key.Binding
	LineStart                  key.Binding
	WordBackward               key.Binding
	WordForward                key.Binding
	InputBegin                 key.Binding
	InputEnd                   key.Binding
	UppercaseWordForward       key.Binding
	LowercaseWordForward       key.Binding
	CapitalizeWordForward      key.Binding
	TransposeCharacterBackward key.Binding
}

// NewKeyMap returns the default key bindings. InsertNewline binds to Ctrl+J
// only; Enter is handled separately via CheckInputComplete.
func NewKeyMap() KeyMap {
	return KeyMap{
		CharacterForward:           key.NewBinding(key.WithKeys("right", "ctrl+f")),
		CharacterBackward:          key.NewBinding(key.WithKeys("left", "ctrl+b")),
		WordForward:                key.NewBinding(key.WithKeys("alt+right", "alt+f")),
		WordBackward:               key.NewBinding(key.WithKeys("alt+left", "alt+b")),
		LineNext:                   key.NewBinding(key.WithKeys("down", "ctrl+n")),
		LinePrevious:               key.NewBinding(key.WithKeys("up", "ctrl+p")),
		DeleteWordBackward:         key.NewBinding(key.WithKeys("alt+backspace", "ctrl+w")),
		DeleteWordForward:          key.NewBinding(key.WithKeys("alt+delete", "alt+d")),
		DeleteAfterCursor:          key.NewBinding(key.WithKeys("ctrl+k")),
		DeleteBeforeCursor:         key.NewBinding(key.WithKeys("ctrl+u")),
		InsertNewline:              key.NewBinding(key.WithKeys("ctrl+j")),
		DeleteCharacterBackward:    key.NewBinding(key.WithKeys("backspace", "ctrl+h")),
		DeleteCharacterForward:     key.NewBinding(key.WithKeys("delete", "ctrl+d")),
		LineStart:                  key.NewBinding(key.WithKeys("home", "ctrl+a")),
		LineEnd:                    key.NewBinding(key.WithKeys("end", "ctrl+e")),
		InputBegin:                 key.NewBinding(key.WithKeys("alt+<", "ctrl+home")),
		InputEnd:                   key.NewBinding(key.WithKeys("alt+>", "ctrl+end")),
		CapitalizeWordForward:      key.NewBinding(key.WithKeys("alt+c")),
		LowercaseWordForward:       key.NewBinding(key.WithKeys("alt+l")),
		UppercaseWordForward:       key.NewBinding(key.WithKeys("alt+u")),
		TransposeCharacterBackward: key.NewBinding(key.WithKeys("ctrl+t")),
	}
}

// LineInfo tracks cursor position within soft-wrapped lines.
type LineInfo struct {
	Width        int
	CharWidth    int
	Height       int
	StartColumn  int
	ColumnOffset int
	RowOffset    int
	CharOffset   int
}

// Model is the textarea model.
type Model struct {
	// CheckInputComplete controls Enter key behavior.
	// When nil or returning false, Enter inserts a newline.
	// When returning true, Enter does nothing (parent handles submission).
	CheckInputComplete func(value string) bool

	// MaxHeight is the maximum height for auto-grow. 0 means no limit.
	MaxHeight int

	// Cursor is the text cursor.
	Cursor cursor.Model

	// KeyMap is the key bindings.
	KeyMap KeyMap

	// CharLimit is the maximum number of characters. 0 means no limit.
	CharLimit int

	// MaxWidth is the maximum width. 0 means no limit.
	MaxWidth int

	cache          *wrapCache
	value          [][]rune
	width          int
	height         int
	focus          bool
	col            int
	row            int
	lastCharOffset int
	viewport       *viewport.Model
	rsan           runeutil.Sanitizer
}

// New creates a new textarea with default settings.
func New() Model {
	vp := viewport.New(0, 0)
	vp.KeyMap = viewport.KeyMap{}

	m := Model{
		MaxHeight: defaultMaxHeight,
		Cursor:    cursor.New(),
		KeyMap:    NewKeyMap(),
		cache:     newWrapCache(),
		value:     make([][]rune, minHeight, maxLines),
		viewport:  &vp,
	}

	m.SetHeight(minHeight)
	m.SetWidth(defaultWidth)
	return m
}

// SetValue sets the textarea content.
func (m *Model) SetValue(s string) {
	m.Reset()
	m.InsertString(s)
}

// Value returns the textarea content.
func (m Model) Value() string {
	if m.value == nil {
		return ""
	}
	var v strings.Builder
	for _, l := range m.value {
		v.WriteString(string(l))
		v.WriteByte('\n')
	}
	return strings.TrimSuffix(v.String(), "\n")
}

// InsertString inserts a string at the cursor position.
func (m *Model) InsertString(s string) {
	m.insertRunesFromUserInput([]rune(s))
}

// InsertRune inserts a rune at the cursor position.
func (m *Model) InsertRune(r rune) {
	m.insertRunesFromUserInput([]rune{r})
}

// Length returns the number of runes in the textarea, including newlines
// between rows. This matches the rune-count semantics used by CharLimit.
func (m Model) Length() int {
	var l int
	for _, row := range m.value {
		l += len(row)
	}
	return l + len(m.value) - 1
}

// LineCount returns the number of lines.
func (m Model) LineCount() int {
	return len(m.value)
}

// Line returns the current line number.
func (m Model) Line() int {
	return m.row
}

// Width returns the width.
func (m Model) Width() int {
	return m.width
}

// Height returns the current height.
func (m Model) Height() int {
	return m.height
}

// Focused returns whether the textarea is focused.
func (m Model) Focused() bool {
	return m.focus
}

// Focus enables keyboard input.
func (m *Model) Focus() tea.Cmd {
	m.focus = true
	return m.Cursor.Focus()
}

// Blur disables keyboard input.
func (m *Model) Blur() {
	m.focus = false
	m.Cursor.Blur()
}

// Reset clears the textarea.
func (m *Model) Reset() {
	m.value = make([][]rune, minHeight, maxLines)
	m.col = 0
	m.row = 0
	m.viewport.GotoTop()
	m.SetCursor(0)
}

// SetWidth sets the textarea width, invalidating the wrap cache.
func (m *Model) SetWidth(w int) {
	inputWidth := max(w, 1)
	if m.MaxWidth > 0 {
		inputWidth = min(inputWidth, m.MaxWidth)
	}
	m.viewport.Width = inputWidth
	m.width = inputWidth
	m.cache.invalidate()
}

// SetHeight sets the textarea height.
func (m *Model) SetHeight(h int) {
	if m.MaxHeight > 0 {
		m.height = clamp(h, minHeight, m.MaxHeight)
		m.viewport.Height = clamp(h, minHeight, m.MaxHeight)
	} else {
		m.height = max(h, minHeight)
		m.viewport.Height = max(h, minHeight)
	}
}

// SetCursor moves the cursor to the given column.
func (m *Model) SetCursor(col int) {
	m.col = clamp(col, 0, len(m.value[m.row]))
	m.lastCharOffset = 0
}

// CursorStart moves the cursor to the start of the line.
func (m *Model) CursorStart() {
	m.SetCursor(0)
}

// CursorEnd moves the cursor to the end of the line.
func (m *Model) CursorEnd() {
	m.SetCursor(len(m.value[m.row]))
}

// CursorDown moves the cursor down by one line.
func (m *Model) CursorDown() {
	li := m.LineInfo()
	charOffset := max(m.lastCharOffset, li.CharOffset)
	m.lastCharOffset = charOffset

	if li.RowOffset+1 >= li.Height && m.row < len(m.value)-1 {
		m.row++
		m.col = 0
	} else {
		const trailingSpace = 2
		m.col = min(li.StartColumn+li.Width+trailingSpace, len(m.value[m.row])-1)
	}

	nli := m.LineInfo()
	m.col = nli.StartColumn

	if nli.Width <= 0 {
		return
	}

	offset := 0
	for offset < charOffset {
		if m.row >= len(m.value) || m.col >= len(m.value[m.row]) || offset >= nli.CharWidth-1 {
			break
		}
		offset += rw.RuneWidth(m.value[m.row][m.col])
		m.col++
	}
}

// CursorUp moves the cursor up by one line.
func (m *Model) CursorUp() {
	li := m.LineInfo()
	charOffset := max(m.lastCharOffset, li.CharOffset)
	m.lastCharOffset = charOffset

	if li.RowOffset <= 0 && m.row > 0 {
		m.row--
		m.col = len(m.value[m.row])
	} else {
		const trailingSpace = 2
		m.col = li.StartColumn - trailingSpace
	}

	nli := m.LineInfo()
	m.col = nli.StartColumn

	if nli.Width <= 0 {
		return
	}

	offset := 0
	for offset < charOffset {
		if m.col >= len(m.value[m.row]) || offset >= nli.CharWidth-1 {
			break
		}
		offset += rw.RuneWidth(m.value[m.row][m.col])
		m.col++
	}
}

// LineInfo returns information about the current soft-wrapped line.
func (m Model) LineInfo() LineInfo {
	grid := m.memoizedWrap(m.value[m.row], m.width)

	var counter int
	for i, line := range grid {
		if counter+len(line) == m.col && i+1 < len(grid) {
			return LineInfo{
				CharOffset:   0,
				ColumnOffset: 0,
				Height:       len(grid),
				RowOffset:    i + 1,
				StartColumn:  m.col,
				Width:        len(grid[i+1]),
				CharWidth:    uniseg.StringWidth(string(line)),
			}
		}

		if counter+len(line) >= m.col {
			return LineInfo{
				CharOffset:   uniseg.StringWidth(string(line[:max(0, m.col-counter)])),
				ColumnOffset: m.col - counter,
				Height:       len(grid),
				RowOffset:    i,
				StartColumn:  counter,
				Width:        len(line),
				CharWidth:    uniseg.StringWidth(string(line)),
			}
		}

		counter += len(line)
	}
	return LineInfo{}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focus {
		m.Cursor.Blur()
		return m, nil
	}

	oldRow, oldCol := m.cursorLineNumber(), m.col
	var cmds []tea.Cmd

	if m.value[m.row] == nil {
		m.value[m.row] = make([]rune, 0)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.KeyMap.DeleteAfterCursor):
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			if m.col >= len(m.value[m.row]) {
				m.mergeLineBelow(m.row)
				break
			}
			m.deleteAfterCursor()
		case key.Matches(msg, m.KeyMap.DeleteBeforeCursor):
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			if m.col <= 0 {
				m.mergeLineAbove(m.row)
				break
			}
			m.deleteBeforeCursor()
		case key.Matches(msg, m.KeyMap.DeleteCharacterBackward):
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			if m.col <= 0 {
				m.mergeLineAbove(m.row)
				break
			}
			if len(m.value[m.row]) > 0 {
				m.value[m.row] = append(m.value[m.row][:max(0, m.col-1)], m.value[m.row][m.col:]...)
				if m.col > 0 {
					m.SetCursor(m.col - 1)
				}
			}
		case key.Matches(msg, m.KeyMap.DeleteCharacterForward):
			if len(m.value[m.row]) > 0 && m.col < len(m.value[m.row]) {
				m.value[m.row] = append(m.value[m.row][:m.col], m.value[m.row][m.col+1:]...)
			}
			if m.col >= len(m.value[m.row]) {
				m.mergeLineBelow(m.row)
				break
			}
		case key.Matches(msg, m.KeyMap.DeleteWordBackward):
			if m.col <= 0 {
				m.mergeLineAbove(m.row)
				break
			}
			m.deleteWordLeft()
		case key.Matches(msg, m.KeyMap.DeleteWordForward):
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			if m.col >= len(m.value[m.row]) {
				m.mergeLineBelow(m.row)
				break
			}
			m.deleteWordRight()
		case key.Matches(msg, m.KeyMap.InsertNewline):
			// Ctrl+J: always insert newline.
			if (m.MaxHeight > 0 && len(m.value) >= m.MaxHeight) || len(m.value) >= maxLines {
				break
			}
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			m.splitLine(m.row, m.col)
		case msg.Type == tea.KeyEnter:
			// Enter: conditional on CheckInputComplete.
			if m.CheckInputComplete != nil && m.CheckInputComplete(m.Value()) {
				break
			}
			if m.MaxHeight > 0 && len(m.value) >= m.MaxHeight {
				break
			}
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			m.splitLine(m.row, m.col)
		case key.Matches(msg, m.KeyMap.LineEnd):
			m.CursorEnd()
		case key.Matches(msg, m.KeyMap.LineStart):
			m.CursorStart()
		case key.Matches(msg, m.KeyMap.CharacterForward):
			m.characterRight()
		case key.Matches(msg, m.KeyMap.LineNext):
			m.CursorDown()
		case key.Matches(msg, m.KeyMap.WordForward):
			m.wordRight()
		case key.Matches(msg, m.KeyMap.CharacterBackward):
			m.characterLeft(false)
		case key.Matches(msg, m.KeyMap.LinePrevious):
			m.CursorUp()
		case key.Matches(msg, m.KeyMap.WordBackward):
			m.wordLeft()
		case key.Matches(msg, m.KeyMap.InputBegin):
			m.moveToBegin()
		case key.Matches(msg, m.KeyMap.InputEnd):
			m.moveToEnd()
		case key.Matches(msg, m.KeyMap.LowercaseWordForward):
			m.lowercaseRight()
		case key.Matches(msg, m.KeyMap.UppercaseWordForward):
			m.uppercaseRight()
		case key.Matches(msg, m.KeyMap.CapitalizeWordForward):
			m.capitalizeRight()
		case key.Matches(msg, m.KeyMap.TransposeCharacterBackward):
			m.transposeLeft()
		default:
			m.insertRunesFromUserInput(msg.Runes)
		}
	}

	if cmd := m.autoGrow(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	vp, cmd := m.viewport.Update(msg)
	m.viewport = &vp
	cmds = append(cmds, cmd)

	newRow, newCol := m.cursorLineNumber(), m.col
	m.Cursor, cmd = m.Cursor.Update(msg)
	if (newRow != oldRow || newCol != oldCol) && m.Cursor.Mode() == cursor.CursorBlink {
		m.Cursor.Blink = false
		cmd = m.Cursor.BlinkCmd()
	}
	cmds = append(cmds, cmd)

	m.repositionView()

	return m, tea.Batch(cmds...)
}

// View renders the textarea.
func (m Model) View() string {
	var s strings.Builder
	lineInfo := m.LineInfo()

	for l, line := range m.value {
		wrappedLines := m.memoizedWrap(line, m.width)

		for wl, wrappedLine := range wrappedLines {
			strwidth := uniseg.StringWidth(string(wrappedLine))
			padding := m.width - strwidth
			if strwidth > m.width {
				wrappedLine = []rune(strings.TrimSuffix(string(wrappedLine), " "))
				padding -= m.width - strwidth
			}

			if m.row == l && lineInfo.RowOffset == wl {
				co := min(lineInfo.ColumnOffset, len(wrappedLine))
				s.WriteString(string(wrappedLine[:co]))
				if co >= len(wrappedLine) || (m.col >= len(line) && lineInfo.CharOffset >= m.width) {
					m.Cursor.SetChar(" ")
					s.WriteString(m.Cursor.View())
				} else {
					m.Cursor.SetChar(string(wrappedLine[co]))
					s.WriteString(m.Cursor.View())
					s.WriteString(string(wrappedLine[co+1:]))
				}
			} else {
				s.WriteString(string(wrappedLine))
			}

			s.WriteString(strings.Repeat(" ", max(0, padding)))
			s.WriteRune('\n')
		}
	}

	m.viewport.SetContent(s.String())
	return m.viewport.View()
}

func (m *Model) autoGrow() tea.Cmd {
	totalLines := m.totalVisibleLines()
	newHeight := totalLines
	if m.MaxHeight > 0 {
		newHeight = min(totalLines, m.MaxHeight)
	}
	newHeight = max(newHeight, minHeight)

	if newHeight != m.height {
		m.SetHeight(newHeight)
		h := newHeight
		return func() tea.Msg {
			return InputHeightMsg{Height: h}
		}
	}
	return nil
}

func (m Model) totalVisibleLines() int {
	total := 0
	for _, line := range m.value {
		total += len(m.memoizedWrap(line, m.width))
	}
	return total
}

func (m Model) memoizedWrap(runes []rune, width int) [][]rune {
	if v, ok := m.cache.get(runes, width); ok {
		return v
	}
	v := wrap(runes, width)
	m.cache.set(runes, width, v)
	return v
}

func (m Model) cursorLineNumber() int {
	ln := 0
	for i := 0; i < m.row; i++ {
		ln += len(m.memoizedWrap(m.value[i], m.width))
	}
	ln += m.LineInfo().RowOffset
	return ln
}

func (m *Model) repositionView() {
	minimum := m.viewport.YOffset
	maximum := minimum + m.viewport.Height - 1

	if row := m.cursorLineNumber(); row < minimum {
		m.viewport.ScrollUp(minimum - row)
	} else if row > maximum {
		m.viewport.ScrollDown(row - maximum)
	}
}

func (m *Model) insertRunesFromUserInput(runes []rune) {
	runes = m.san().Sanitize(runes)

	if m.CharLimit > 0 {
		availSpace := m.CharLimit - m.Length()
		if availSpace <= 0 {
			return
		}
		if availSpace < len(runes) {
			runes = runes[:availSpace]
		}
	}

	var lines [][]rune
	lstart := 0
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\n' {
			lines = append(lines, runes[lstart:i:i])
			lstart = i + 1
		}
	}
	if lstart <= len(runes) {
		lines = append(lines, runes[lstart:])
	}

	if maxLines > 0 && len(m.value)+len(lines)-1 > maxLines {
		allowedHeight := max(0, maxLines-len(m.value)+1)
		lines = lines[:allowedHeight]
	}

	if len(lines) == 0 {
		return
	}

	tail := make([]rune, len(m.value[m.row][m.col:]))
	copy(tail, m.value[m.row][m.col:])

	m.value[m.row] = append(m.value[m.row][:m.col], lines[0]...)
	m.col += len(lines[0])

	if numExtraLines := len(lines) - 1; numExtraLines > 0 {
		var newGrid [][]rune
		if cap(m.value) >= len(m.value)+numExtraLines {
			newGrid = m.value[:len(m.value)+numExtraLines]
		} else {
			newGrid = make([][]rune, len(m.value)+numExtraLines)
			copy(newGrid, m.value[:m.row+1])
		}
		copy(newGrid[m.row+1+numExtraLines:], m.value[m.row+1:])
		m.value = newGrid
		for _, l := range lines[1:] {
			m.row++
			m.value[m.row] = l
			m.col = len(l)
		}
	}

	m.value[m.row] = append(m.value[m.row], tail...)
	m.SetCursor(m.col)
}

func (m *Model) san() runeutil.Sanitizer {
	if m.rsan == nil {
		m.rsan = runeutil.NewSanitizer()
	}
	return m.rsan
}

func (m *Model) splitLine(row, col int) {
	// Copy both halves to avoid aliasing the original backing array.
	headSrc, tailSrc := m.value[row][:col], m.value[row][col:]
	head := make([]rune, len(headSrc))
	copy(head, headSrc)
	tail := make([]rune, len(tailSrc))
	copy(tail, tailSrc)

	m.value = append(m.value[:row+1], m.value[row:]...)
	m.value[row] = head
	m.value[row+1] = tail

	m.col = 0
	m.row++
}

func (m *Model) mergeLineBelow(row int) {
	if row >= len(m.value)-1 {
		return
	}
	m.value[row] = append(m.value[row], m.value[row+1]...)
	for i := row + 1; i < len(m.value)-1; i++ {
		m.value[i] = m.value[i+1]
	}
	if len(m.value) > 0 {
		m.value = m.value[:len(m.value)-1]
	}
}

func (m *Model) mergeLineAbove(row int) {
	if row <= 0 {
		return
	}
	m.col = len(m.value[row-1])
	m.row--
	m.value[row-1] = append(m.value[row-1], m.value[row]...)
	for i := row; i < len(m.value)-1; i++ {
		m.value[i] = m.value[i+1]
	}
	if len(m.value) > 0 {
		m.value = m.value[:len(m.value)-1]
	}
}

func (m *Model) deleteBeforeCursor() {
	m.value[m.row] = m.value[m.row][m.col:]
	m.SetCursor(0)
}

func (m *Model) deleteAfterCursor() {
	m.value[m.row] = m.value[m.row][:m.col]
	m.SetCursor(len(m.value[m.row]))
}

func (m *Model) transposeLeft() {
	if m.col == 0 || len(m.value[m.row]) < 2 {
		return
	}
	if m.col >= len(m.value[m.row]) {
		m.SetCursor(m.col - 1)
	}
	m.value[m.row][m.col-1], m.value[m.row][m.col] = m.value[m.row][m.col], m.value[m.row][m.col-1]
	if m.col < len(m.value[m.row]) {
		m.SetCursor(m.col + 1)
	}
}

func (m *Model) deleteWordLeft() {
	if m.col == 0 || len(m.value[m.row]) == 0 {
		return
	}
	oldCol := m.col
	m.SetCursor(m.col - 1)
	for unicode.IsSpace(m.value[m.row][m.col]) {
		if m.col <= 0 {
			break
		}
		m.SetCursor(m.col - 1)
	}
	for m.col > 0 {
		if !unicode.IsSpace(m.value[m.row][m.col]) {
			m.SetCursor(m.col - 1)
		} else {
			if m.col > 0 {
				m.SetCursor(m.col + 1)
			}
			break
		}
	}
	if oldCol > len(m.value[m.row]) {
		m.value[m.row] = m.value[m.row][:m.col]
	} else {
		m.value[m.row] = append(m.value[m.row][:m.col], m.value[m.row][oldCol:]...)
	}
}

func (m *Model) deleteWordRight() {
	if m.col >= len(m.value[m.row]) || len(m.value[m.row]) == 0 {
		return
	}
	oldCol := m.col
	for m.col < len(m.value[m.row]) && unicode.IsSpace(m.value[m.row][m.col]) {
		m.SetCursor(m.col + 1)
	}
	for m.col < len(m.value[m.row]) {
		if !unicode.IsSpace(m.value[m.row][m.col]) {
			m.SetCursor(m.col + 1)
		} else {
			break
		}
	}
	if m.col > len(m.value[m.row]) {
		m.value[m.row] = m.value[m.row][:oldCol]
	} else {
		m.value[m.row] = append(m.value[m.row][:oldCol], m.value[m.row][m.col:]...)
	}
	m.SetCursor(oldCol)
}

func (m *Model) characterRight() {
	if m.col < len(m.value[m.row]) {
		m.SetCursor(m.col + 1)
	} else if m.row < len(m.value)-1 {
		m.row++
		m.CursorStart()
	}
}

func (m *Model) characterLeft(insideLine bool) {
	if m.col == 0 && m.row != 0 {
		m.row--
		m.CursorEnd()
		if !insideLine {
			return
		}
	}
	if m.col > 0 {
		m.SetCursor(m.col - 1)
	}
}

func (m *Model) wordLeft() {
	for {
		m.characterLeft(true)
		if m.col < len(m.value[m.row]) && !unicode.IsSpace(m.value[m.row][m.col]) {
			break
		}
	}
	for m.col > 0 {
		if unicode.IsSpace(m.value[m.row][m.col-1]) {
			break
		}
		m.SetCursor(m.col - 1)
	}
}

func (m *Model) wordRight() {
	m.doWordRight(func(int, int) {})
}

func (m *Model) doWordRight(fn func(charIdx int, pos int)) {
	for m.col >= len(m.value[m.row]) || unicode.IsSpace(m.value[m.row][m.col]) {
		if m.row == len(m.value)-1 && m.col == len(m.value[m.row]) {
			break
		}
		m.characterRight()
	}
	charIdx := 0
	for m.col < len(m.value[m.row]) {
		if unicode.IsSpace(m.value[m.row][m.col]) {
			break
		}
		fn(charIdx, m.col)
		m.SetCursor(m.col + 1)
		charIdx++
	}
}

func (m *Model) uppercaseRight() {
	m.doWordRight(func(_ int, i int) {
		m.value[m.row][i] = unicode.ToUpper(m.value[m.row][i])
	})
}

func (m *Model) lowercaseRight() {
	m.doWordRight(func(_ int, i int) {
		m.value[m.row][i] = unicode.ToLower(m.value[m.row][i])
	})
}

func (m *Model) capitalizeRight() {
	m.doWordRight(func(charIdx int, i int) {
		if charIdx == 0 {
			m.value[m.row][i] = unicode.ToTitle(m.value[m.row][i])
		}
	})
}

func (m *Model) moveToBegin() {
	m.row = 0
	m.SetCursor(0)
}

func (m *Model) moveToEnd() {
	m.row = len(m.value) - 1
	m.SetCursor(len(m.value[m.row]))
}

func clamp(v, low, high int) int {
	if high < low {
		low, high = high, low
	}
	return min(high, max(low, v))
}
