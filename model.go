package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- messages ---

type reportMsg Report
type tickMsg time.Time

// --- model ---

type model struct {
	width     int
	height    int
	loading   bool
	spinner   spinner.Model
	report    *Report
	startedAt time.Time
	scroll    int
}

func initialModel() model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)
	return model{
		loading:   true,
		spinner:   sp,
		startedAt: time.Now(),
		width:     100,
	}
}

func collectCmd() tea.Cmd {
	return func() tea.Msg {
		return reportMsg(collect())
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, collectCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case reportMsg:
		r := Report(msg)
		m.report = &r
		m.loading = false
		return m, nil

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			if !m.loading {
				m.loading = true
				m.report = nil
				m.startedAt = time.Now()
				m.scroll = 0
				return m, tea.Batch(m.spinner.Tick, collectCmd())
			}
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
			return m, nil
		case "down", "j":
			m.scroll++
			return m, nil
		case "pgup":
			m.scroll -= 10
			if m.scroll < 0 {
				m.scroll = 0
			}
			return m, nil
		case "pgdown", " ":
			m.scroll += 10
			return m, nil
		case "home", "g":
			m.scroll = 0
			return m, nil
		case "end", "G":
			m.scroll = 1 << 30
			return m, nil
		}
	}
	return m, nil
}

func (m model) View() string {
	w := m.width
	if w == 0 {
		w = 100
	}
	h := m.height
	if h == 0 {
		h = 30
	}

	if m.loading {
		elapsed := time.Since(m.startedAt).Round(100 * time.Millisecond)
		body := fmt.Sprintf("%s %s",
			m.spinner.View(),
			titleStyle.Render("Gathering system metrics..."))
		hint := mutedStyle.Render(fmt.Sprintf(
			"Running df / docker / ps / journalctl. Elapsed: %s", elapsed))
		content := body + "\n\n" + hint
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
	}
	if m.report == nil {
		return mutedStyle.Render("No report.")
	}

	full := renderReport(*m.report, w)
	lines := strings.Split(full, "\n")
	total := len(lines)

	top, topLines := renderTopBar(*m.report, w)
	bottomLines := 3 // separator + shortcuts + status

	contentH := h - topLines - bottomLines
	if contentH < 3 {
		contentH = 3
	}

	maxScroll := total - contentH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	if m.scroll < 0 {
		m.scroll = 0
	}

	end := m.scroll + contentH
	if end > total {
		end = total
	}
	visible := append([]string(nil), lines[m.scroll:end]...)
	for len(visible) < contentH {
		visible = append(visible, "")
	}

	bottom := renderBottomBar(m.scroll, end, total, w)

	return top + "\n" + strings.Join(visible, "\n") + "\n" + bottom
}

// renderTopBar produces the always-visible header. Returns the rendered
// string (including the trailing separator) and the total number of lines it
// occupies (2 when it fits on one row, 3 when it wraps on narrow terminals).
func renderTopBar(r Report, width int) (string, int) {
	name := accentStyle.Render("sina")
	host := titleStyle.Render(r.Hostname)
	bdg := badge(r.Decision.Overall)
	when := mutedStyle.Render(r.Collected.Format("2006-01-02 15:04:05"))
	dot := mutedStyle.Render("  ·  ")
	sepLine := mutedStyle.Render(strings.Repeat("─", width))

	full := name + dot + host + " " + bdg + dot + when
	if lipgloss.Width(full) <= width {
		return full + "\n" + sepLine, 2
	}

	// Wrap: keep identity (name + host + status) on the first line, push the
	// timestamp to a second line so nothing important gets dropped.
	line1 := name + dot + host + " " + bdg
	if lipgloss.Width(line1) <= width {
		return line1 + "\n" + when + "\n" + sepLine, 3
	}

	// Even narrower: drop the branding, keep host + status badge.
	line1 = host + " " + bdg
	return line1 + "\n" + when + "\n" + sepLine, 3
}

// renderBottomBar is the always-visible footer: separator, shortcuts row, and
// scroll-state row. Always 3 lines so the viewport size is stable while
// scrolling. The scroll-state row is the cue first-time users need to realize
// the view is scrollable.
func renderBottomBar(start, end, total, width int) string {
	sepLine := mutedStyle.Render(strings.Repeat("─", width))
	return sepLine + "\n" + renderShortcutLine(width) + "\n" + renderStatusLine(start, end, total, width)
}

func renderShortcutLine(width int) string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	items := []struct{ k, l string }{
		{"q", "quit"},
		{"r", "refresh"},
		{"↑/↓", "scroll"},
		{"space", "page"},
		{"g/G", "top/end"},
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, keyStyle.Render(it.k)+" "+mutedStyle.Render(it.l))
	}
	bar := strings.Join(parts, mutedStyle.Render("  ·  "))
	if lipgloss.Width(bar) <= width {
		return bar
	}

	// Compact form for narrow terminals.
	parts = parts[:0]
	for _, it := range items[:4] {
		parts = append(parts, keyStyle.Render(it.k)+" "+mutedStyle.Render(it.l))
	}
	return strings.Join(parts, mutedStyle.Render(" · "))
}

func renderStatusLine(start, end, total, width int) string {
	var hint string
	switch {
	case total == 0:
		hint = mutedStyle.Render("(no content)")
	case start == 0 && end >= total:
		hint = mutedStyle.Render("(report fits on screen)")
	case end >= total:
		hint = statusText(StatusOK, "▽ END — at bottom")
	default:
		more := total - end
		hint = statusText(StatusInfo,
			fmt.Sprintf("▼ %d more line(s) below — press ↓ / space to scroll", more))
	}

	pos := mutedStyle.Render(fmt.Sprintf("%d–%d / %d", start+1, end, total))

	pad := width - lipgloss.Width(hint) - lipgloss.Width(pos)
	if pad < 1 {
		// Try a shorter hint to make room for the position counter.
		if total > 0 && end < total {
			hint = statusText(StatusInfo, fmt.Sprintf("▼ %d more", total-end))
		}
		pad = width - lipgloss.Width(hint) - lipgloss.Width(pos)
		if pad < 1 {
			pad = 1
		}
	}
	return hint + strings.Repeat(" ", pad) + pos
}
