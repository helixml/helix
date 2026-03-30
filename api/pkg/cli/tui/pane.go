package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// SplitDir is the direction of a split.
type SplitDir int

const (
	SplitVertical   SplitDir = iota // left | right
	SplitHorizontal                 // top / bottom
)

// PaneTree is a binary tree of panes. Each node is either a leaf (chat view)
// or a split containing two children.
type PaneTree struct {
	// Leaf fields (set when this is a leaf node)
	Chat *ChatModel
	ID   int // unique pane ID for focus tracking

	// Split fields (set when this is a split node)
	Dir   SplitDir
	Left  *PaneTree // or Top
	Right *PaneTree // or Bottom
	Ratio float64   // 0.0-1.0, proportion given to Left/Top
}

// PaneManager manages the pane tree and focus.
type PaneManager struct {
	Root    *PaneTree
	focused int // ID of the focused pane
	nextID  int
	width   int
	height  int
}

func NewPaneManager() *PaneManager {
	return &PaneManager{
		nextID: 1,
	}
}

func (pm *PaneManager) SetSize(w, h int) {
	pm.width = w
	pm.height = h
}

// OpenPane creates the first pane or adds to existing layout.
func (pm *PaneManager) OpenPane(chat *ChatModel) {
	pane := &PaneTree{
		Chat:  chat,
		ID:    pm.nextID,
		Ratio: 0.5,
	}
	pm.nextID++

	if pm.Root == nil {
		pm.Root = pane
	}
	pm.focused = pane.ID
	chat.SetSize(pm.width, pm.height)
}

// SplitFocused splits the currently focused pane in the given direction,
// placing the new chat in the new pane.
func (pm *PaneManager) SplitFocused(dir SplitDir, chat *ChatModel) {
	if pm.Root == nil {
		pm.OpenPane(chat)
		return
	}

	newPane := &PaneTree{
		Chat:  chat,
		ID:    pm.nextID,
		Ratio: 0.5,
	}
	pm.nextID++

	leaf := pm.findPane(pm.Root, pm.focused)
	if leaf == nil {
		return
	}

	// Replace the leaf with a split containing the old leaf and new pane
	oldChat := leaf.Chat
	oldID := leaf.ID

	leaf.Chat = nil
	leaf.Dir = dir
	leaf.Ratio = 0.5
	leaf.Left = &PaneTree{Chat: oldChat, ID: oldID, Ratio: 0.5}
	leaf.Right = newPane

	pm.focused = newPane.ID
	pm.recalcSizes()
}

// CloseFocused closes the focused pane.  Returns false if no panes remain.
func (pm *PaneManager) CloseFocused() bool {
	if pm.Root == nil {
		return false
	}
	// Single pane
	if pm.Root.Chat != nil {
		pm.Root = nil
		return false
	}

	parent, isLeft := pm.findParent(pm.Root, pm.focused)
	if parent == nil {
		return false
	}

	// Replace parent with the sibling
	var sibling *PaneTree
	if isLeft {
		sibling = parent.Right
	} else {
		sibling = parent.Left
	}

	parent.Chat = sibling.Chat
	parent.ID = sibling.ID
	parent.Dir = sibling.Dir
	parent.Left = sibling.Left
	parent.Right = sibling.Right
	parent.Ratio = sibling.Ratio

	// Focus the remaining pane
	pm.focused = pm.firstLeafID(parent)
	pm.recalcSizes()
	return true
}

// FocusNext cycles focus to the next pane.
func (pm *PaneManager) FocusNext() {
	leaves := pm.allLeaves(pm.Root)
	for i, l := range leaves {
		if l.ID == pm.focused {
			next := (i + 1) % len(leaves)
			pm.focused = leaves[next].ID
			return
		}
	}
}

// FocusPrev cycles focus to the previous pane.
func (pm *PaneManager) FocusPrev() {
	leaves := pm.allLeaves(pm.Root)
	for i, l := range leaves {
		if l.ID == pm.focused {
			prev := (i - 1 + len(leaves)) % len(leaves)
			pm.focused = leaves[prev].ID
			return
		}
	}
}

// FocusedChat returns the chat model of the focused pane.
func (pm *PaneManager) FocusedChat() *ChatModel {
	if pm.Root == nil {
		return nil
	}
	leaf := pm.findPane(pm.Root, pm.focused)
	if leaf == nil {
		return nil
	}
	return leaf.Chat
}

// PaneCount returns the number of leaf panes.
func (pm *PaneManager) PaneCount() int {
	return len(pm.allLeaves(pm.Root))
}

// IsEmpty returns true if there are no panes.
func (pm *PaneManager) IsEmpty() bool {
	return pm.Root == nil
}

// Render renders the pane tree to a string.
func (pm *PaneManager) Render() string {
	if pm.Root == nil {
		return ""
	}
	return pm.renderNode(pm.Root, pm.width, pm.height)
}

func (pm *PaneManager) renderNode(node *PaneTree, w, h int) string {
	if node.Chat != nil {
		// Leaf node — render the chat
		node.Chat.SetSize(w, h)
		isFocused := node.ID == pm.focused

		content := node.Chat.View()

		if isFocused {
			borderStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).
				Width(w - 2).
				Height(h - 2)
			return borderStyle.Render(content)
		}

		borderStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Width(w - 2).
			Height(h - 2)
		return borderStyle.Render(content)
	}

	// Split node
	switch node.Dir {
	case SplitVertical:
		leftW := int(float64(w) * node.Ratio)
		rightW := w - leftW
		if leftW < 4 {
			leftW = 4
		}
		if rightW < 4 {
			rightW = 4
		}
		left := pm.renderNode(node.Left, leftW, h)
		right := pm.renderNode(node.Right, rightW, h)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	case SplitHorizontal:
		topH := int(float64(h) * node.Ratio)
		bottomH := h - topH
		if topH < 3 {
			topH = 3
		}
		if bottomH < 3 {
			bottomH = 3
		}
		top := pm.renderNode(node.Left, w, topH)
		bottom := pm.renderNode(node.Right, w, bottomH)
		return top + "\n" + bottom

	default:
		return ""
	}
}

func (pm *PaneManager) recalcSizes() {
	// Sizes are calculated dynamically during render, nothing to do here
}

func (pm *PaneManager) findPane(node *PaneTree, id int) *PaneTree {
	if node == nil {
		return nil
	}
	if node.Chat != nil {
		if node.ID == id {
			return node
		}
		return nil
	}
	if found := pm.findPane(node.Left, id); found != nil {
		return found
	}
	return pm.findPane(node.Right, id)
}

func (pm *PaneManager) findParent(node *PaneTree, id int) (parent *PaneTree, isLeft bool) {
	if node == nil || node.Chat != nil {
		return nil, false
	}
	if node.Left != nil && node.Left.ID == id && node.Left.Chat != nil {
		return node, true
	}
	if node.Right != nil && node.Right.ID == id && node.Right.Chat != nil {
		return node, false
	}
	// Check if it's a leaf inside a subtree
	if pm.containsLeaf(node.Left, id) {
		return pm.findParent(node.Left, id)
	}
	if pm.containsLeaf(node.Right, id) {
		return pm.findParent(node.Right, id)
	}
	return nil, false
}

func (pm *PaneManager) containsLeaf(node *PaneTree, id int) bool {
	if node == nil {
		return false
	}
	if node.Chat != nil {
		return node.ID == id
	}
	return pm.containsLeaf(node.Left, id) || pm.containsLeaf(node.Right, id)
}

func (pm *PaneManager) firstLeafID(node *PaneTree) int {
	if node == nil {
		return 0
	}
	if node.Chat != nil {
		return node.ID
	}
	return pm.firstLeafID(node.Left)
}

func (pm *PaneManager) allLeaves(node *PaneTree) []*PaneTree {
	if node == nil {
		return nil
	}
	if node.Chat != nil {
		return []*PaneTree{node}
	}
	left := pm.allLeaves(node.Left)
	right := pm.allLeaves(node.Right)
	return append(left, right...)
}

