package main

import "github.com/charmbracelet/lipgloss"

var (
	colorOK      = lipgloss.AdaptiveColor{Light: "#16a34a", Dark: "#22c55e"}
	colorWarn    = lipgloss.AdaptiveColor{Light: "#b45309", Dark: "#f59e0b"}
	colorBad     = lipgloss.AdaptiveColor{Light: "#b91c1c", Dark: "#ef4444"}
	colorInfo    = lipgloss.AdaptiveColor{Light: "#0e7490", Dark: "#06b6d4"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#64748b", Dark: "#94a3b8"}
	colorBorder  = lipgloss.AdaptiveColor{Light: "#cbd5e1", Dark: "#475569"}
	colorTitle   = lipgloss.AdaptiveColor{Light: "#0f172a", Dark: "#f1f5f9"}
	colorAccent  = lipgloss.AdaptiveColor{Light: "#7c3aed", Dark: "#a78bfa"}
	colorBarFill = lipgloss.AdaptiveColor{Light: "#0f172a", Dark: "#e2e8f0"}
)

func statusColor(s Status) lipgloss.AdaptiveColor {
	switch s {
	case StatusOK:
		return colorOK
	case StatusWarn:
		return colorWarn
	case StatusBad:
		return colorBad
	case StatusInfo:
		return colorInfo
	}
	return colorMuted
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTitle)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	accentStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTitle).
			Padding(0, 1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(1, 0)
)

func badge(s Status) string {
	st := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#ffffff")).
		Background(statusColor(s)).
		Padding(0, 1)
	return st.Render(" " + s.Label() + " ")
}

func statusText(s Status, text string) string {
	return lipgloss.NewStyle().Foreground(statusColor(s)).Render(text)
}

func boxWithTitle(title string, body string, width int) string {
	titleBar := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Render(title)
	content := titleBar + "\n" + body
	style := boxStyle
	if width > 0 {
		style = style.Width(width)
	}
	return style.Render(content)
}

// barString renders a unicode progress bar of given width filled to pct (0-100).
func barString(pct float64, width int, s Status) string {
	if width < 6 {
		width = 6
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	fill := int(float64(width) * pct / 100.0)
	if fill > width {
		fill = width
	}
	full := "█"
	empty := "░"
	left := lipgloss.NewStyle().Foreground(statusColor(s)).Render(repeat(full, fill))
	right := lipgloss.NewStyle().Foreground(colorMuted).Render(repeat(empty, width-fill))
	return left + right
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
