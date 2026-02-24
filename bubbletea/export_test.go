package bubbletea

// BlockSeparator exports blockSeparator for testing.
func BlockSeparator(prev, curr MessageBlock) string {
	return blockSeparator(prev, curr)
}

// RenderContent exports renderContent for testing.
func RenderContent(m Model) string {
	return m.renderContent()
}

// AllExpanded returns whether all collapsible blocks are in expanded state.
func AllExpanded(m Model) bool {
	return m.allExpanded
}
