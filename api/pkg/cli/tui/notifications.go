package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// NotificationType categorizes notifications.
type NotificationType int

const (
	NotifySpecReady NotificationType = iota
	NotifyImplComplete
	NotifyPRCreated
	NotifyAgentError
	NotifyTaskStatusChange
	NotifyInfo
)

// Notification is a single notification item.
type Notification struct {
	ID        string
	Type      NotificationType
	Title     string
	Detail    string
	TaskID    string
	Timestamp time.Time
	Read      bool
}

// NotificationManager tracks and renders notifications.
type NotificationManager struct {
	mu            sync.Mutex
	notifications []*Notification
	visible       bool // whether the notification list is shown
	cursor        int
}

func NewNotificationManager() *NotificationManager {
	return &NotificationManager{}
}

// Add adds a notification.
func (nm *NotificationManager) Add(n *Notification) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.notifications = append([]*Notification{n}, nm.notifications...)
}

// UnreadCount returns the number of unread notifications.
func (nm *NotificationManager) UnreadCount() int {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	count := 0
	for _, n := range nm.notifications {
		if !n.Read {
			count++
		}
	}
	return count
}

// UnreadForTask returns the unread count for a specific task.
func (nm *NotificationManager) UnreadForTask(taskID string) int {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	count := 0
	for _, n := range nm.notifications {
		if !n.Read && n.TaskID == taskID {
			count++
		}
	}
	return count
}

// MarkRead marks all notifications as read.
func (nm *NotificationManager) MarkAllRead() {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	for _, n := range nm.notifications {
		n.Read = true
	}
}

// MarkTaskRead marks notifications for a specific task as read.
func (nm *NotificationManager) MarkTaskRead(taskID string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	for _, n := range nm.notifications {
		if n.TaskID == taskID {
			n.Read = true
		}
	}
}

// Toggle shows/hides the notification list.
func (nm *NotificationManager) Toggle() {
	nm.visible = !nm.visible
	if nm.visible {
		nm.cursor = 0
	}
}

// IsVisible returns whether the list is shown.
func (nm *NotificationManager) IsVisible() bool {
	return nm.visible
}

// Render renders the notification list as a modal overlay.
func (nm *NotificationManager) Render(width, height int) string {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	modalWidth := width * 2 / 3
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > 100 {
		modalWidth = 100
	}

	var b strings.Builder

	unread := 0
	for _, n := range nm.notifications {
		if !n.Read {
			unread++
		}
	}

	title := styleHeader.Render(fmt.Sprintf("Notifications (%d unread)", unread))
	b.WriteString(title + "\n\n")

	if len(nm.notifications) == 0 {
		b.WriteString(styleDim.Render("  No notifications"))
	} else {
		maxVisible := height - 8
		if maxVisible < 3 {
			maxVisible = 3
		}
		for i := 0; i < maxVisible && i < len(nm.notifications); i++ {
			n := nm.notifications[i]
			b.WriteString(nm.renderNotification(n, i == nm.cursor, modalWidth-4))
			b.WriteString("\n")
		}
		if len(nm.notifications) > maxVisible {
			b.WriteString(styleDim.Render(fmt.Sprintf("  +%d more", len(nm.notifications)-maxVisible)))
		}
	}

	b.WriteString("\n" + styleDim.Render("  j/k: navigate  enter: go to task  m: mark all read  esc: close"))

	content := b.String()
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Width(modalWidth).
		Padding(1, 2).
		Render(content)

	// Center
	padTop := (height - 15) / 2
	padLeft := (width - modalWidth - 2) / 2
	if padTop < 0 {
		padTop = 0
	}
	if padLeft < 0 {
		padLeft = 0
	}

	lines := strings.Split(modal, "\n")
	var result []string
	for i := 0; i < padTop; i++ {
		result = append(result, "")
	}
	for _, line := range lines {
		result = append(result, strings.Repeat(" ", padLeft)+line)
	}

	return strings.Join(result, "\n")
}

func (nm *NotificationManager) renderNotification(n *Notification, selected bool, width int) string {
	icon := nm.typeIcon(n.Type)
	age := timeAgo(n.Timestamp)

	unreadMark := "  "
	if !n.Read {
		unreadMark = lipgloss.NewStyle().Foreground(colorPrimary).Render("● ")
	}

	line := fmt.Sprintf("%s%s %s  %s",
		unreadMark, icon, truncate(n.Title, width-20), styleDim.Render(age))

	if selected {
		return lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(colorText).
			Width(width).
			Render(line)
	}
	return "  " + line
}

func (nm *NotificationManager) typeIcon(t NotificationType) string {
	switch t {
	case NotifySpecReady:
		return lipgloss.NewStyle().Foreground(colorPrimary).Render("📋")
	case NotifyImplComplete:
		return lipgloss.NewStyle().Foreground(colorSuccess).Render("✓")
	case NotifyPRCreated:
		return lipgloss.NewStyle().Foreground(colorSuccess).Render("⤴")
	case NotifyAgentError:
		return lipgloss.NewStyle().Foreground(colorError).Render("✗")
	case NotifyTaskStatusChange:
		return lipgloss.NewStyle().Foreground(colorWarning).Render("→")
	default:
		return styleDim.Render("·")
	}
}
