package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// version is injected at build time via -ldflags="-X main.version=..."
var version = "dev"

func main() {
	var (
		plain   = flag.Bool("plain", false, "Print plain (non-interactive) report and exit")
		noColor = flag.Bool("no-color", false, "Disable colors")
		showVer = flag.Bool("version", false, "Show version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("sina", version)
		return
	}

	if *noColor {
		os.Setenv("NO_COLOR", "1")
		os.Setenv("CLICOLOR", "0")
	}

	// Auto-fallback to plain mode when stdout is not a TTY (piping, scripting).
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		*plain = true
	}

	if *plain {
		runPlain()
		return
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runPlain() {
	r := collect()
	width := 100
	if term.IsTerminal(int(os.Stdout.Fd())) {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			width = w
		}
	}
	fmt.Println(renderHeader(r, width))
	fmt.Println(renderReport(r, width))

	// Exit code reflects severity for scripting use.
	switch r.Decision.Overall {
	case StatusBad:
		os.Exit(2)
	case StatusWarn:
		os.Exit(1)
	}
}
