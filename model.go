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
	if m.loading {
		elapsed := time.Since(m.startedAt).Round(100 * time.Millisecond)
		body := fmt.Sprintf("%s %s",
			m.spinner.View(),
			titleStyle.Render("Gathering system metrics..."))
		hint := mutedStyle.Render(fmt.Sprintf(
			"\nRunning df / docker / ps / journalctl. Elapsed: %s\n", elapsed))
		return "\n" + body + hint
	}
	if m.report == nil {
		return mutedStyle.Render("No report.")
	}

	w := m.width
	if w == 0 {
		w = 100
	}
	full := renderReport(*m.report, w)

	// Footer
	footer := footerStyle.Render(
		"[q] quit   [r] refresh   [↑/↓] scroll   [space] page down")
	full = full + "\n" + footer

	// Apply scroll: viewport height = m.height - 1 (footer baked in already).
	if m.height > 0 {
		lines := strings.Split(full, "\n")
		if m.scroll > len(lines)-1 {
			m.scroll = max(0, len(lines)-3)
		}
		visible := m.height
		if visible <= 0 {
			visible = len(lines)
		}
		end := m.scroll + visible
		if end > len(lines) {
			end = len(lines)
		}
		return strings.Join(lines[m.scroll:end], "\n")
	}
	return full
}
