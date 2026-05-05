package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderReport renders the full report at the given terminal width. width=0
// means "use default 100".
func renderReport(r Report, width int) string {
	if width <= 0 {
		width = 100
	}
	if width < 60 {
		width = 60
	}

	var b strings.Builder

	// Two-column layout for system + memory if width allows.
	colW := (width - 4) / 2
	if width >= 100 {
		left := renderSystemBox(r, colW)
		right := renderMemoryBox(r, colW)
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right))
		b.WriteString("\n")
	} else {
		b.WriteString(renderSystemBox(r, width-2))
		b.WriteString("\n")
		b.WriteString(renderMemoryBox(r, width-2))
		b.WriteString("\n")
	}

	b.WriteString(renderDiskBox(r, width-2))
	b.WriteString("\n")

	if r.Docker != nil {
		b.WriteString(renderDockerBox(r, width-2))
		b.WriteString("\n")
	}

	b.WriteString(renderProcessesBox(r, width-2))
	b.WriteString("\n")

	b.WriteString(renderLogsBox(r, width-2))
	b.WriteString("\n")

	b.WriteString(renderHealthExtras(r, width-2))
	b.WriteString("\n")

	b.WriteString(renderDecisionBox(r, width-2))
	return b.String()
}

func renderHeader(r Report, width int) string {
	left := accentStyle.Render("sina")
	host := titleStyle.Render(r.Hostname)
	when := mutedStyle.Render(r.Collected.Format("2006-01-02 15:04:05"))
	line1 := fmt.Sprintf("%s   %s   %s", left, host, when)

	osStr := r.OS
	if osStr == "" {
		osStr = "Linux"
	}
	line2 := mutedStyle.Render(fmt.Sprintf(
		"uptime: %s   kernel: %s   %s",
		r.Uptime, r.Kernel, osStr,
	))
	body := line1 + "\n" + line2
	return boxStyle.Width(width - 2).Render(body)
}

func renderSystemBox(r Report, width int) string {
	s := r.System
	var b strings.Builder
	fmt.Fprintf(&b, "Cores:     %d\n", s.Cores)
	fmt.Fprintf(&b, "Load 1m:   %.2f  (%s)\n", s.Load1, statusText(s.Status, fmt.Sprintf("%.0f%%", s.LoadPct)))
	fmt.Fprintf(&b, "Load 5m:   %.2f\n", s.Load5)
	fmt.Fprintf(&b, "Load 15m:  %.2f\n", s.Load15)
	innerW := width - 6
	if innerW < 10 {
		innerW = 10
	}
	fmt.Fprintf(&b, "%s\n", barString(s.LoadPct, innerW, s.Status))
	if s.Note != "" {
		b.WriteString(mutedStyle.Render(s.Note) + "\n")
	}
	return sectionBox("CPU & Load", s.Status, b.String(), width)
}

func renderMemoryBox(r Report, width int) string {
	m := r.Memory
	var b strings.Builder
	fmt.Fprintf(&b, "Total:     %s\n", humanMB(m.TotalMB))
	fmt.Fprintf(&b, "Used:      %s  (%.0f%%)\n", humanMB(m.UsedMB), m.UsedPct)
	fmt.Fprintf(&b, "Available: %s  (%s)\n", humanMB(m.AvailableMB),
		statusText(m.Status, fmt.Sprintf("%.0f%%", m.AvailPct)))
	if m.SwapTotalMB > 0 {
		fmt.Fprintf(&b, "Swap:      %s / %s  (%s)\n",
			humanMB(m.SwapUsedMB), humanMB(m.SwapTotalMB),
			statusText(m.SwapStatus, fmt.Sprintf("%.0f%%", m.SwapPct)))
	} else {
		fmt.Fprintf(&b, "Swap:      none\n")
	}
	innerW := width - 6
	if innerW < 10 {
		innerW = 10
	}
	fmt.Fprintf(&b, "%s\n", barString(m.UsedPct, innerW, m.Status))
	if m.Note != "" {
		b.WriteString(mutedStyle.Render(m.Note) + "\n")
	}
	return sectionBox("Memory", m.Status, b.String(), width)
}

func renderDiskBox(r Report, width int) string {
	d := r.Disk
	var b strings.Builder
	fmt.Fprintf(&b, "Root /:    %.1f / %.1f GB used  (%s)   inodes: %s\n",
		d.UsedGB, d.TotalGB,
		statusText(d.Status, fmt.Sprintf("%.0f%%", d.UsedPct)),
		statusText(r.Inodes.Status, fmt.Sprintf("%.0f%%", r.Inodes.UsedPct)),
	)
	innerW := width - 6
	if innerW < 10 {
		innerW = 10
	}
	fmt.Fprintf(&b, "%s\n", barString(d.UsedPct, innerW, d.Status))

	if len(d.Mounts) > 1 {
		b.WriteString("\n" + mutedStyle.Render("Other mounts:") + "\n")
		for _, m := range d.Mounts {
			if m.Path == "/" {
				continue
			}
			fmt.Fprintf(&b, "  %-30s %6s / %-6s  %s\n",
				truncate(m.Path, 30), m.Used, m.Total,
				statusText(m.Status, fmt.Sprintf("%.0f%%", m.UsedPct)))
		}
	}

	if len(d.BigDirs) > 0 {
		b.WriteString("\n" + mutedStyle.Render("Disk-heavy directories under /:") + "\n")
		for _, dir := range d.BigDirs {
			fmt.Fprintf(&b, "  %6s  %s\n", dir.Size, dir.Path)
		}
	} else {
		b.WriteString("\n" + mutedStyle.Render("(rerun with sudo to see top-level disk usage)") + "\n")
	}

	if d.Note != "" {
		b.WriteString(mutedStyle.Render(d.Note) + "\n")
	}
	return sectionBox("Disk & Inodes", d.Status, b.String(), width)
}

func renderDockerBox(r Report, width int) string {
	doc := r.Docker
	if !doc.Available {
		body := mutedStyle.Render(doc.Reason)
		return sectionBox("Docker", StatusInfo, body, width)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Containers: %d running, %d stopped, %d unhealthy, %d restarting\n",
		doc.Running, doc.Stopped, doc.Unhealthy, doc.Restarting)
	fmt.Fprintf(&b, "Images:     %d (%.2f GB)   Volumes: %.2f GB   BuildCache: %.2f GB\n",
		doc.Images, doc.ImagesGB, doc.VolumesGB, doc.BuildCacheGB)
	fmt.Fprintf(&b, "Reclaimable: %s\n",
		statusText(doc.Status, fmt.Sprintf("~%.2f GB", doc.ReclaimGB)))

	if len(doc.UnhealthyList) > 0 {
		b.WriteString("\n" + statusText(StatusBad, "Unhealthy:") + " " +
			strings.Join(doc.UnhealthyList, ", ") + "\n")
	}

	if len(doc.RestartLoops) > 0 {
		b.WriteString("\n" + statusText(StatusBad, "Restart loops:") + "\n")
		for _, rl := range doc.RestartLoops {
			var detail string
			switch {
			case rl.RatePerMin > 0:
				detail = fmt.Sprintf("%.1f/min · total %d", rl.RatePerMin, rl.RestartCount)
			default:
				detail = fmt.Sprintf("total %d · state=%s", rl.RestartCount, rl.State)
			}
			fmt.Fprintf(&b, "  %-30s %s   %s\n",
				truncate(rl.Name, 30),
				statusText(rl.Status, detail),
				mutedStyle.Render("("+rl.Reason+")"))
		}
	}

	if len(doc.TopCPU) > 0 || len(doc.TopMem) > 0 {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Top by CPU                          Top by Mem") + "\n")
		rows := max(len(doc.TopCPU), len(doc.TopMem))
		for i := 0; i < rows; i++ {
			leftCol := ""
			rightCol := ""
			if i < len(doc.TopCPU) {
				c := doc.TopCPU[i]
				leftCol = fmt.Sprintf("  %-25s %5.1f%%", truncate(c.Name, 25), c.CPUPct)
			}
			if i < len(doc.TopMem) {
				c := doc.TopMem[i]
				rightCol = fmt.Sprintf("  %-25s %s", truncate(c.Name, 25), c.MemRaw)
			}
			fmt.Fprintf(&b, "%-36s %s\n", leftCol, rightCol)
		}
	}

	if doc.Note != "" {
		b.WriteString(mutedStyle.Render(doc.Note) + "\n")
	}
	return sectionBox("Docker", doc.Status, b.String(), width)
}

func renderProcessesBox(r Report, width int) string {
	var b strings.Builder
	b.WriteString(mutedStyle.Render("Top by CPU") + "\n")
	for _, p := range r.Processes.TopCPU {
		fmt.Fprintf(&b, "  %-8s %-12s %5.1f%% CPU  %5.1f%% MEM  %s\n",
			p.PID, truncate(p.User, 12), p.CPUPct, p.MemPct, p.Cmd)
	}
	b.WriteString("\n" + mutedStyle.Render("Top by Memory") + "\n")
	for _, p := range r.Processes.TopMem {
		fmt.Fprintf(&b, "  %-8s %-12s %5.1f%% MEM  %7.0f MB  %s\n",
			p.PID, truncate(p.User, 12), p.MemPct, p.RSSMB, p.Cmd)
	}
	if len(r.Zombies.Procs) > 0 {
		b.WriteString("\n" + statusText(StatusWarn, fmt.Sprintf("Zombie processes (%d):", len(r.Zombies.Procs))) + "\n")
		for _, z := range r.Zombies.Procs {
			fmt.Fprintf(&b, "  pid=%s ppid=%s cmd=%s\n", z.PID, z.PPID, z.Cmd)
		}
	}
	return sectionBox("Processes", StatusInfo, b.String(), width)
}

func renderLogsBox(r Report, width int) string {
	var b strings.Builder
	if r.Logs.JournalSize != "" {
		b.WriteString(r.Logs.JournalSize + "\n")
	}
	if len(r.Logs.BigLogDirs) > 0 {
		b.WriteString(mutedStyle.Render("Largest entries under /var/log:") + "\n")
		for _, d := range r.Logs.BigLogDirs {
			fmt.Fprintf(&b, "  %6s  %s\n", d.Size, d.Path)
		}
	}
	return sectionBox("Logs", StatusInfo, b.String(), width)
}

func renderHealthExtras(r Report, width int) string {
	var b strings.Builder
	rows := []struct {
		label string
		val   string
		st    Status
	}{
		{"OOM kills (24h)", fmt.Sprintf("%d", r.System2.OOMKills24h),
			tern(r.System2.OOMKills24h > 0, StatusWarn, StatusOK)},
		{"Failed systemd units", fmt.Sprintf("%d", r.System2.FailedUnits),
			tern(r.System2.FailedUnits > 0, StatusWarn, StatusOK)},
		{"Reboot required", tern(r.System2.RebootRequired, "yes ("+r.System2.RebootReason+")", "no"),
			tern(r.System2.RebootRequired, StatusInfo, StatusOK)},
		{"Established TCP connections", fmt.Sprintf("%d", r.System2.Connections), StatusInfo},
		{"File descriptors", fmt.Sprintf("%d / %d (%.0f%%)", r.FDs.Used, r.FDs.Max, r.FDs.UsedPct), r.FDs.Status},
	}
	for _, row := range rows {
		fmt.Fprintf(&b, "  %-30s %s\n", row.label, statusText(row.st, row.val))
	}
	if len(r.System2.FailedUnitList) > 0 {
		b.WriteString("\n" + mutedStyle.Render("Failed units: "+
			strings.Join(r.System2.FailedUnitList, ", ")) + "\n")
	}
	return sectionBox("System Health", StatusInfo, b.String(), width)
}

func renderDecisionBox(r Report, width int) string {
	d := r.Decision
	var b strings.Builder
	headline := lipgloss.NewStyle().
		Bold(true).
		Foreground(statusColor(d.Overall)).
		Render(d.Overall.Symbol() + "  " + d.Headline)
	b.WriteString(headline + "\n")

	if len(d.Reasons) > 0 {
		b.WriteString("\n" + mutedStyle.Render("Why:") + "\n")
		for _, r := range d.Reasons {
			fmt.Fprintf(&b, "  • %s\n", r)
		}
	}
	if len(d.Actions) > 0 {
		b.WriteString("\n" + mutedStyle.Render("Suggested actions:") + "\n")
		for _, a := range d.Actions {
			fmt.Fprintf(&b, "  → %s\n", a)
		}
	}
	if len(d.Reasons) == 0 && len(d.Actions) == 0 {
		b.WriteString("\n" + mutedStyle.Render("All checks passed within thresholds.") + "\n")
	}
	return sectionBox("Diagnosis", d.Overall, b.String(), width)
}

// sectionBox is a titled, status-colored box.
func sectionBox(title string, s Status, body string, width int) string {
	t := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Render(title)
	bdg := badge(s)
	header := t + "  " + bdg
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(statusColor(s)).
		Padding(0, 1)
	if width > 0 {
		style = style.Width(width)
	}
	return style.Render(header + "\n" + strings.TrimRight(body, "\n"))
}

// ---- helpers ----

func humanMB(mb uint64) string {
	if mb >= 1024 {
		return fmt.Sprintf("%.2f GB", float64(mb)/1024)
	}
	return fmt.Sprintf("%d MB", mb)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func tern[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
