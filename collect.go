package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Thresholds (tunable in one place).
var thresholds = struct {
	CPUWarnPct   float64
	CPUBadPct    float64
	MemAvailWarn float64
	MemAvailBad  float64
	SwapWarnMB   uint64
	DiskWarnPct  float64
	DiskBadPct   float64
	InodeWarnPct float64
	InodeBadPct  float64
	FDWarnPct    float64
	FDBadPct     float64
	OOMWarn      int
	UnitsWarn    int
	// Container restart-loop thresholds (per-minute restart rate).
	RestartWarnPerMin float64
	RestartBadPerMin  float64
	// Per-run absolute count thresholds (for first-run / no-cache detection).
	RestartCountSuspicious int
	JustRestartedSec       float64
}{
	CPUWarnPct:             70,  // sustained busy
	CPUBadPct:              100, // saturated
	MemAvailWarn:           20,
	MemAvailBad:            10,
	SwapWarnMB:             512,
	DiskWarnPct:            75,
	DiskBadPct:             90,
	InodeWarnPct:           85,
	InodeBadPct:            95,
	FDWarnPct:              70,
	FDBadPct:               90,
	OOMWarn:                1,
	UnitsWarn:              1,
	RestartWarnPerMin:      10,
	RestartBadPerMin:       100,
	RestartCountSuspicious: 100,
	JustRestartedSec:       60,
}

// runCmd runs a binary with a hard timeout and returns combined output.
func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runShell(timeout time.Duration, script string) (string, error) {
	return runCmd(timeout, "bash", "-c", script)
}

// collect gathers everything. Each section runs defensively — failures show as
// errors in the report but do not abort the run.
func collect() Report {
	r := Report{Collected: time.Now()}

	if h, err := os.Hostname(); err == nil {
		r.Hostname = h
	}
	if out, err := runCmd(2*time.Second, "uname", "-r"); err == nil {
		r.Kernel = strings.TrimSpace(out)
	}
	if out, err := runShell(2*time.Second, ". /etc/os-release && echo \"$PRETTY_NAME\""); err == nil {
		r.OS = strings.TrimSpace(out)
	}
	r.Uptime, r.BootTime = collectUptime()
	r.System = collectSystem()
	r.Memory = collectMemory()
	r.Disk = collectDisk()
	r.Inodes = collectInodes()
	r.FDs = collectFDs()
	r.Docker = collectDocker()
	r.Logs = collectLogs()
	r.Processes = collectProcesses()
	r.Zombies = collectZombies()
	r.System2 = collectExtraSystem()

	r.Decision = decide(&r)
	return r
}

// ---------- uptime ----------

func collectUptime() (string, time.Time) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "unknown", time.Time{}
	}
	parts := strings.Fields(string(data))
	if len(parts) == 0 {
		return "unknown", time.Time{}
	}
	secs, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return "unknown", time.Time{}
	}
	d := time.Duration(secs) * time.Second
	boot := time.Now().Add(-d)
	return humanizeDuration(d), boot
}

func humanizeDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

// ---------- system / cpu ----------

func collectSystem() SystemSection {
	s := SystemSection{Cores: runtime.NumCPU()}
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		s.Status = StatusUnknown
		s.Note = "could not read /proc/loadavg"
		return s
	}
	f := strings.Fields(string(data))
	if len(f) >= 3 {
		s.Load1, _ = strconv.ParseFloat(f[0], 64)
		s.Load5, _ = strconv.ParseFloat(f[1], 64)
		s.Load15, _ = strconv.ParseFloat(f[2], 64)
	}
	if s.Cores > 0 {
		s.LoadPct = s.Load1 / float64(s.Cores) * 100
	}
	switch {
	case s.LoadPct >= thresholds.CPUBadPct:
		s.Status = StatusBad
		s.Note = "load exceeds cores — CPU saturated, consider an upgrade"
	case s.LoadPct >= thresholds.CPUWarnPct:
		s.Status = StatusWarn
		s.Note = "load is sustained-busy"
	default:
		s.Status = StatusOK
	}
	return s
}

// ---------- memory ----------

func collectMemory() MemorySection {
	m := MemorySection{}
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		m.Status = StatusUnknown
		m.Note = "could not read /proc/meminfo"
		return m
	}
	vals := map[string]uint64{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := line[:colon]
		rest := strings.TrimSpace(line[colon+1:])
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = v // values are in KB
	}
	toMB := func(kb uint64) uint64 { return kb / 1024 }
	m.TotalMB = toMB(vals["MemTotal"])
	m.FreeMB = toMB(vals["MemFree"])
	m.AvailableMB = toMB(vals["MemAvailable"])
	m.BuffersMB = toMB(vals["Buffers"])
	m.CachedMB = toMB(vals["Cached"])
	if m.TotalMB > m.AvailableMB {
		m.UsedMB = m.TotalMB - m.AvailableMB
	}
	if m.TotalMB > 0 {
		m.UsedPct = float64(m.UsedMB) / float64(m.TotalMB) * 100
		m.AvailPct = float64(m.AvailableMB) / float64(m.TotalMB) * 100
	}
	m.SwapTotalMB = toMB(vals["SwapTotal"])
	swapFree := toMB(vals["SwapFree"])
	if m.SwapTotalMB > swapFree {
		m.SwapUsedMB = m.SwapTotalMB - swapFree
	}
	if m.SwapTotalMB > 0 {
		m.SwapPct = float64(m.SwapUsedMB) / float64(m.SwapTotalMB) * 100
	}

	switch {
	case m.AvailPct <= thresholds.MemAvailBad:
		m.Status = StatusBad
		m.Note = "available RAM critical — RAM upgrade recommended"
	case m.AvailPct <= thresholds.MemAvailWarn:
		m.Status = StatusWarn
		m.Note = "available RAM low — investigate memory hogs"
	default:
		m.Status = StatusOK
	}

	switch {
	case m.SwapUsedMB >= thresholds.SwapWarnMB*2:
		m.SwapStatus = StatusBad
	case m.SwapUsedMB >= thresholds.SwapWarnMB:
		m.SwapStatus = StatusWarn
	default:
		m.SwapStatus = StatusOK
	}
	return m
}

// ---------- disk ----------

func collectDisk() DiskSection {
	d := DiskSection{Mount: "/"}
	out, err := runCmd(3*time.Second, "df", "-PB1", "/")
	if err != nil {
		d.Status = StatusUnknown
		d.Note = "df / failed: " + strings.TrimSpace(out)
		return d
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) >= 2 {
		f := strings.Fields(lines[1])
		if len(f) >= 5 {
			total, _ := strconv.ParseFloat(f[1], 64)
			used, _ := strconv.ParseFloat(f[2], 64)
			avail, _ := strconv.ParseFloat(f[3], 64)
			d.TotalGB = total / 1024 / 1024 / 1024
			d.UsedGB = used / 1024 / 1024 / 1024
			d.AvailGB = avail / 1024 / 1024 / 1024
			pct := strings.TrimSuffix(f[4], "%")
			d.UsedPct, _ = strconv.ParseFloat(pct, 64)
		}
	}
	switch {
	case d.UsedPct >= thresholds.DiskBadPct:
		d.Status = StatusBad
		d.Note = "root almost full — cleanup or disk upgrade required"
	case d.UsedPct >= thresholds.DiskWarnPct:
		d.Status = StatusWarn
		d.Note = "root usage high — cleanup recommended"
	default:
		d.Status = StatusOK
	}

	d.BigDirs = collectBigDirs("/")
	d.Mounts = collectMounts()
	return d
}

func collectMounts() []MountInfo {
	out, err := runCmd(3*time.Second, "df", "-PHT")
	if err != nil {
		return nil
	}
	var result []MountInfo
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 7 {
			continue
		}
		fstype := f[1]
		// skip pseudo / virtual / overlay filesystems
		switch fstype {
		case "tmpfs", "devtmpfs", "squashfs", "overlay", "proc", "sysfs", "cgroup", "cgroup2", "fuse.snapfuse", "ramfs":
			continue
		}
		mount := f[6]
		if strings.HasPrefix(mount, "/snap") || strings.HasPrefix(mount, "/var/lib/docker") {
			continue
		}
		pct, _ := strconv.ParseFloat(strings.TrimSuffix(f[5], "%"), 64)
		mi := MountInfo{
			Path:    mount,
			Total:   f[2],
			Used:    f[3],
			UsedPct: pct,
		}
		switch {
		case pct >= thresholds.DiskBadPct:
			mi.Status = StatusBad
		case pct >= thresholds.DiskWarnPct:
			mi.Status = StatusWarn
		default:
			mi.Status = StatusOK
		}
		result = append(result, mi)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UsedPct > result[j].UsedPct })
	return result
}

func collectBigDirs(root string) []DirSize {
	// Try without sudo first; if that fails (permission noise), try sudo -n.
	script := fmt.Sprintf(
		"du -xh --max-depth=1 %s 2>/dev/null | sort -h | tail -12",
		shellQuote(root),
	)
	out, _ := runShell(20*time.Second, script)
	if strings.TrimSpace(out) == "" {
		out, _ = runShell(20*time.Second,
			"sudo -n "+script+" 2>/dev/null")
	}
	return parseDuLines(out)
}

func parseDuLines(out string) []DirSize {
	var dirs []DirSize
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		dirs = append(dirs, DirSize{Size: f[0], Path: strings.Join(f[1:], " ")})
	}
	return dirs
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ---------- inodes ----------

func collectInodes() InodeSection {
	out, err := runCmd(3*time.Second, "df", "-iP", "/")
	if err != nil {
		return InodeSection{Status: StatusUnknown}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return InodeSection{Status: StatusUnknown}
	}
	f := strings.Fields(lines[1])
	if len(f) < 5 {
		return InodeSection{Status: StatusUnknown}
	}
	pct, _ := strconv.ParseFloat(strings.TrimSuffix(f[4], "%"), 64)
	is := InodeSection{UsedPct: pct}
	switch {
	case pct >= thresholds.InodeBadPct:
		is.Status = StatusBad
	case pct >= thresholds.InodeWarnPct:
		is.Status = StatusWarn
	default:
		is.Status = StatusOK
	}
	return is
}

// ---------- file descriptors ----------

func collectFDs() FDSection {
	data, err := os.ReadFile("/proc/sys/fs/file-nr")
	if err != nil {
		return FDSection{Status: StatusUnknown}
	}
	f := strings.Fields(string(data))
	if len(f) < 3 {
		return FDSection{Status: StatusUnknown}
	}
	used, _ := strconv.Atoi(f[0])
	max, _ := strconv.Atoi(f[2])
	fd := FDSection{Used: used, Max: max}
	if max > 0 {
		fd.UsedPct = float64(used) / float64(max) * 100
	}
	switch {
	case fd.UsedPct >= thresholds.FDBadPct:
		fd.Status = StatusBad
	case fd.UsedPct >= thresholds.FDWarnPct:
		fd.Status = StatusWarn
	default:
		fd.Status = StatusOK
	}
	return fd
}

// ---------- docker ----------

func dockerInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

func collectDocker() *DockerSection {
	d := &DockerSection{}
	if !dockerInstalled() {
		d.Available = false
		d.Reason = "docker binary not found"
		return d
	}
	// Probe with a quick info call. If this fails we still return the section
	// but mark unavailable.
	if _, err := runCmd(4*time.Second, "docker", "info", "--format", "{{.ID}}"); err != nil {
		d.Available = false
		d.Reason = "cannot talk to dockerd (permission or daemon down)"
		return d
	}
	d.Available = true

	collectDockerCounts(d)
	collectDockerSystemDF(d)
	collectDockerStats(d)
	collectDockerRestarts(d)

	// Final status: pick the most severe finding and explain it once.
	d.Status = StatusOK
	hasBadRestart := false
	hasWarnRestart := false
	for _, rl := range d.RestartLoops {
		if rl.Status >= StatusBad {
			hasBadRestart = true
		}
		if rl.Status >= StatusWarn {
			hasWarnRestart = true
		}
	}
	switch {
	case hasBadRestart:
		d.Status = StatusBad
		d.Note = fmt.Sprintf("restart loop detected on %d container(s)", len(d.RestartLoops))
	case d.Unhealthy > 0:
		d.Status = StatusWarn
		d.Note = fmt.Sprintf("%d unhealthy container(s)", d.Unhealthy)
	case hasWarnRestart:
		d.Status = StatusWarn
		d.Note = fmt.Sprintf("frequent restarts on %d container(s)", len(d.RestartLoops))
	case d.ReclaimGB >= 5:
		d.Status = StatusWarn
		d.Note = fmt.Sprintf("~%.1f GB reclaimable — consider 'docker system prune'", d.ReclaimGB)
	}
	return d
}

func collectDockerCounts(d *DockerSection) {
	out, err := runCmd(4*time.Second, "docker", "ps", "-a",
		"--format", "{{.Names}}\t{{.State}}\t{{.Status}}")
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		state := f[1]
		status := f[2]
		switch state {
		case "running":
			d.Running++
		case "restarting":
			d.Restarting++
		default:
			d.Stopped++
		}
		if strings.Contains(status, "(unhealthy)") {
			d.Unhealthy++
			d.UnhealthyList = append(d.UnhealthyList, f[0])
		}
	}
}

func collectDockerSystemDF(d *DockerSection) {
	out, err := runCmd(5*time.Second, "docker", "system", "df", "--format", "{{json .}}")
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var row struct {
			Type        string
			TotalCount  string
			Size        string
			Reclaimable string
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		sizeGB := parseSizeGB(row.Size)
		reclaimGB := parseReclaimableGB(row.Reclaimable)
		switch row.Type {
		case "Images":
			d.Images, _ = strconv.Atoi(row.TotalCount)
			d.ImagesGB = sizeGB
			d.ReclaimGB += reclaimGB
		case "Containers":
			d.ReclaimGB += reclaimGB
		case "Local Volumes":
			d.VolumesGB = sizeGB
			d.ReclaimGB += reclaimGB
		case "Build Cache":
			d.BuildCacheGB = sizeGB
			d.ReclaimGB += reclaimGB
		}
	}
}

// parseSizeGB accepts strings like "1.234GB", "512MB", "0B" and returns GB.
func parseSizeGB(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0B" {
		return 0
	}
	// Find split between number and unit.
	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' || c == ',' {
			i++
			continue
		}
		break
	}
	num, err := strconv.ParseFloat(strings.ReplaceAll(s[:i], ",", ""), 64)
	if err != nil {
		return 0
	}
	unit := strings.ToUpper(strings.TrimSpace(s[i:]))
	switch unit {
	case "B":
		return num / 1024 / 1024 / 1024
	case "KB":
		return num / 1024 / 1024
	case "MB":
		return num / 1024
	case "GB":
		return num
	case "TB":
		return num * 1024
	}
	return 0
}

func parseReclaimableGB(s string) float64 {
	// docker prints "1.2GB (50%)" — strip the parenthetical.
	if idx := strings.Index(s, "("); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	return parseSizeGB(s)
}

func collectDockerStats(d *DockerSection) {
	out, err := runCmd(8*time.Second, "docker", "stats", "--no-stream",
		"--format", "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}")
	if err != nil {
		return
	}
	var rows []DockerContainer
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		cpu, _ := strconv.ParseFloat(strings.TrimSuffix(f[1], "%"), 64)
		// MemUsage looks like "12.34MiB / 1.5GiB"
		memRaw := f[2]
		memUsed := strings.TrimSpace(strings.SplitN(memRaw, "/", 2)[0])
		memMB := parseMemMB(memUsed)
		rows = append(rows, DockerContainer{
			Name:   f[0],
			CPUPct: cpu,
			MemMB:  memMB,
			MemRaw: memUsed,
		})
	}
	cpuRows := append([]DockerContainer(nil), rows...)
	sort.Slice(cpuRows, func(i, j int) bool { return cpuRows[i].CPUPct > cpuRows[j].CPUPct })
	memRows := append([]DockerContainer(nil), rows...)
	sort.Slice(memRows, func(i, j int) bool { return memRows[i].MemMB > memRows[j].MemMB })
	cap5 := func(in []DockerContainer) []DockerContainer {
		if len(in) > 5 {
			return in[:5]
		}
		return in
	}
	d.TopCPU = cap5(cpuRows)
	d.TopMem = cap5(memRows)
}

// collectDockerRestarts detects containers stuck in a restart loop. It
// combines two signals so that even a first run (with no cache) catches the
// pathological case where a container has thousands of restarts and just
// (re)started seconds ago — the GlitchTip-on-Coolify scenario.
//
//  1. State == "restarting"               → BAD (the daemon itself reports it)
//  2. Cache delta → restart rate per min  → WARN (>10/min) / BAD (>100/min)
//  3. RestartCount > N AND Up < 60s       → BAD (heuristic, no cache needed)
//
// The cache is best-effort: read failures are ignored, write failures are
// ignored. A stale cache (>24h) is also ignored to avoid bogus rates.
func collectDockerRestarts(d *DockerSection) {
	idsOut, err := runCmd(4*time.Second, "docker", "ps", "-a", "--no-trunc", "-q")
	if err != nil {
		return
	}
	ids := strings.Fields(strings.TrimSpace(idsOut))
	if len(ids) == 0 {
		return
	}

	args := append([]string{
		"inspect",
		"--format",
		"{{.Id}}\t{{.Name}}\t{{.State.Status}}\t{{.RestartCount}}\t{{.State.StartedAt}}",
	}, ids...)
	out, err := runCmd(8*time.Second, "docker", args...)
	if err != nil {
		return
	}

	type currentContainer struct {
		ID, Name, State string
		RestartCount    int
		StartedAt       time.Time
	}
	var current []currentContainer
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			continue
		}
		rc, _ := strconv.Atoi(strings.TrimSpace(f[3]))
		startedAt, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(f[4]))
		current = append(current, currentContainer{
			ID:           strings.TrimSpace(f[0]),
			Name:         strings.TrimPrefix(strings.TrimSpace(f[1]), "/"),
			State:        strings.TrimSpace(f[2]),
			RestartCount: rc,
			StartedAt:    startedAt,
		})
	}

	prev, _ := readContainersCache()
	now := time.Now()

	// Persist current state for the next run, regardless of what we flag now.
	newCache := containersCache{
		CapturedAt: now,
		Containers: make(map[string]cachedContainer, len(current)),
	}
	for _, c := range current {
		newCache.Containers[c.ID] = cachedContainer{
			RestartCount: c.RestartCount,
			CapturedAt:   now,
		}
	}
	_ = writeContainersCache(&newCache)

	for _, c := range current {
		var (
			st        Status
			reason    string
			delta     int
			rate      float64
			uptimeSec float64
		)
		if !c.StartedAt.IsZero() && c.State == "running" {
			uptimeSec = now.Sub(c.StartedAt).Seconds()
		}

		if c.State == "restarting" {
			st = StatusBad
			reason = "state=restarting"
		}
		if st < StatusBad &&
			c.RestartCount > thresholds.RestartCountSuspicious &&
			c.State == "running" &&
			uptimeSec > 0 && uptimeSec < thresholds.JustRestartedSec {
			st = StatusBad
			reason = fmt.Sprintf("restart_count=%d, just (re)started %.0fs ago",
				c.RestartCount, uptimeSec)
		}
		if prev != nil {
			if cached, ok := prev.Containers[c.ID]; ok {
				elapsed := now.Sub(cached.CapturedAt)
				if elapsed > 0 && elapsed < 24*time.Hour {
					delta = c.RestartCount - cached.RestartCount
					if delta > 0 {
						rate = float64(delta) / elapsed.Minutes()
						switch {
						case rate > thresholds.RestartBadPerMin && st < StatusBad:
							st = StatusBad
							reason = fmt.Sprintf("%.0f restarts/min (Δ%d in %s)",
								rate, delta, elapsed.Round(time.Second))
						case rate > thresholds.RestartWarnPerMin && st < StatusWarn:
							st = StatusWarn
							reason = fmt.Sprintf("%.1f restarts/min (Δ%d in %s)",
								rate, delta, elapsed.Round(time.Second))
						}
					}
				}
			}
		}

		if st >= StatusWarn {
			d.RestartLoops = append(d.RestartLoops, ContainerRestart{
				Name:         c.Name,
				State:        c.State,
				RestartCount: c.RestartCount,
				DeltaCount:   delta,
				RatePerMin:   rate,
				UptimeSec:    uptimeSec,
				Status:       st,
				Reason:       reason,
			})
		}
	}

	sort.Slice(d.RestartLoops, func(i, j int) bool {
		return d.RestartLoops[i].Status > d.RestartLoops[j].Status
	})
}

// --- container restart-count cache ---

type cachedContainer struct {
	RestartCount int       `json:"restart_count"`
	CapturedAt   time.Time `json:"captured_at"`
}

type containersCache struct {
	CapturedAt time.Time                  `json:"captured_at"`
	Containers map[string]cachedContainer `json:"containers"`
}

func containersCachePath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "vps-health", "containers.json"), nil
}

func readContainersCache() (*containersCache, error) {
	p, err := containersCachePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var c containersCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func writeContainersCache(c *containersCache) error {
	p, err := containersCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// parseMemMB parses Docker mem strings like "12.34MiB", "1.5GiB", "200KiB" into MB.
func parseMemMB(s string) float64 {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' {
			i++
			continue
		}
		break
	}
	if i == 0 {
		return 0
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	unit := strings.ToLower(strings.TrimSpace(s[i:]))
	switch unit {
	case "b":
		return num / 1024 / 1024
	case "kb", "kib":
		return num / 1024
	case "mb", "mib":
		return num
	case "gb", "gib":
		return num * 1024
	case "tb", "tib":
		return num * 1024 * 1024
	}
	return 0
}

// ---------- logs ----------

func collectLogs() LogsSection {
	l := LogsSection{}
	if out, err := runCmd(4*time.Second, "journalctl", "--disk-usage"); err == nil {
		l.JournalSize = strings.TrimSpace(out)
	}
	out, _ := runShell(10*time.Second, "du -sh /var/log/* 2>/dev/null | sort -h | tail -10")
	l.BigLogDirs = parseDuLines(out)
	return l
}

// ---------- processes ----------

func collectProcesses() ProcessesSection {
	p := ProcessesSection{}
	p.TopCPU = topProcs("-%cpu")
	p.TopMem = topProcs("-%mem")
	return p
}

func topProcs(sortKey string) []ProcessRow {
	out, err := runCmd(4*time.Second, "ps", "-eo",
		"pid,user,%cpu,%mem,rss,comm", "--sort", sortKey)
	if err != nil {
		return nil
	}
	var rows []ProcessRow
	scanner := bufio.NewScanner(strings.NewReader(out))
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		f := strings.Fields(scanner.Text())
		if len(f) < 6 {
			continue
		}
		cpu, _ := strconv.ParseFloat(f[2], 64)
		mem, _ := strconv.ParseFloat(f[3], 64)
		rssKB, _ := strconv.ParseFloat(f[4], 64)
		rows = append(rows, ProcessRow{
			PID:    f[0],
			User:   f[1],
			CPUPct: cpu,
			MemPct: mem,
			RSSMB:  rssKB / 1024,
			Cmd:    strings.Join(f[5:], " "),
		})
		if len(rows) >= 6 {
			break
		}
	}
	return rows
}

// ---------- zombies ----------

func collectZombies() ZombiesSection {
	out, err := runCmd(3*time.Second, "ps", "-eo", "pid,ppid,stat,comm")
	if err != nil {
		return ZombiesSection{Status: StatusUnknown}
	}
	z := ZombiesSection{Status: StatusOK}
	scanner := bufio.NewScanner(strings.NewReader(out))
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		f := strings.Fields(scanner.Text())
		if len(f) < 4 {
			continue
		}
		if strings.Contains(f[2], "Z") {
			z.Procs = append(z.Procs, ZombieProc{
				PID: f[0], PPID: f[1], Cmd: strings.Join(f[3:], " "),
			})
		}
	}
	if len(z.Procs) > 0 {
		z.Status = StatusWarn
	}
	return z
}

// ---------- extra system ----------

func collectExtraSystem() ExtraSystem {
	e := ExtraSystem{}

	// OOM kills in last 24h via journalctl (best-effort; falls back silently).
	if out, err := runShell(5*time.Second,
		"journalctl --since '24 hours ago' -k 2>/dev/null | grep -ci 'killed process\\|out of memory'"); err == nil {
		e.OOMKills24h, _ = strconv.Atoi(strings.TrimSpace(out))
	}

	// Failed systemd units.
	if out, err := runCmd(3*time.Second, "systemctl", "--failed", "--no-legend"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" {
				continue
			}
			f := strings.Fields(line)
			if len(f) > 0 {
				e.FailedUnits++
				e.FailedUnitList = append(e.FailedUnitList, f[0])
			}
		}
	}

	// Reboot required (Ubuntu marker).
	if _, err := os.Stat("/var/run/reboot-required"); err == nil {
		e.RebootRequired = true
		if data, err := os.ReadFile("/var/run/reboot-required.pkgs"); err == nil {
			pkgs := strings.Fields(string(data))
			if len(pkgs) > 0 {
				e.RebootReason = fmt.Sprintf("%d package(s) need a restart", len(pkgs))
			}
		}
		if e.RebootReason == "" {
			e.RebootReason = "kernel/library update pending"
		}
	}

	// Connection count (TCP established).
	if out, err := runShell(3*time.Second, "ss -tan state established 2>/dev/null | wc -l"); err == nil {
		n, _ := strconv.Atoi(strings.TrimSpace(out))
		if n > 0 {
			e.Connections = n - 1 // header line
		}
	}

	return e
}
