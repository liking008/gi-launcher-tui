package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// item is a selectable menu entry.
type item struct {
	title, desc string
	key         string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title + " " + i.desc }

type itemDelegate struct{}

func (itemDelegate) Height() int                         { return 2 }
func (itemDelegate) Spacing() int                        { return 1 }
func (itemDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (itemDelegate) Render(w io.Writer, m list.Model, idx int, li list.Item) {
	i, ok := li.(item)
	if !ok {
		return
	}
	sel := idx == m.Index()
	title := i.title
	desc := i.desc
	if sel {
		title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("> " + title)
		desc = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("  " + desc)
	} else {
		title = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render("  " + title)
		desc = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  " + desc)
	}
	line := title + "\n" + desc
	if sel {
		line = lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("205")).
			PaddingLeft(1).Render(line)
	}
	w.Write([]byte(line))
}

func titleStyle(s string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Padding(0, 1).Render(s)
}
func accentStyle(s string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")).Render(s)
}
func infoStyle(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(s)
}
func errStyle(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(s)
}
func hintStyle(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(s)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
