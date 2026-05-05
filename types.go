package main

import "time"

// Status represents a check level.
type Status int

const (
	StatusOK Status = iota
	StatusWarn
	StatusBad
	StatusInfo
	StatusUnknown
)

func (s Status) Label() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusWarn:
		return "WARN"
	case StatusBad:
		return "BAD"
	case StatusInfo:
		return "INFO"
	default:
		return "?"
	}
}

func (s Status) Symbol() string {
	switch s {
	case StatusOK:
		return "✓"
	case StatusWarn:
		return "!"
	case StatusBad:
		return "✗"
	case StatusInfo:
		return "•"
	default:
		return "·"
	}
}

// Report is the full health snapshot.
type Report struct {
	Hostname  string
	Kernel    string
	OS        string
	Uptime    string
	BootTime  time.Time
	Collected time.Time

	System    SystemSection
	Memory    MemorySection
	Disk      DiskSection
	Inodes    InodeSection
	FDs       FDSection
	Docker    *DockerSection
	Logs      LogsSection
	Processes ProcessesSection
	Zombies   ZombiesSection
	System2   ExtraSystem // OOMs, failed units, reboot required, network

	Decision  Decision
	SinceLast SinceLastSection
	Errors    []string
}

type SystemSection struct {
	Cores   int
	Load1   float64
	Load5   float64
	Load15  float64
	LoadPct float64 // Load1 / Cores * 100
	Status  Status
	Note    string
}

type MemorySection struct {
	TotalMB        uint64
	UsedMB         uint64
	FreeMB         uint64
	AvailableMB    uint64
	BuffersMB      uint64
	CachedMB       uint64
	UsedPct        float64
	AvailPct       float64
	SwapTotalMB    uint64
	SwapUsedMB     uint64
	SwapPct        float64
	SwapInRateKBs  float64 // pages swapped IN per second × pageSize / 1024
	SwapOutRateKBs float64 // pages swapped OUT per second × pageSize / 1024
	SwapMeasured   bool    // true once we have a previous sample to diff against
	Status         Status
	SwapStatus     Status
	Note           string
	SwapNote       string
}

type DiskSection struct {
	Mount   string
	TotalGB float64
	UsedGB  float64
	AvailGB float64
	UsedPct float64
	Status  Status
	Note    string
	BigDirs []DirSize
	Mounts  []MountInfo
}

type MountInfo struct {
	Path    string
	UsedPct float64
	Total   string
	Used    string
	Status  Status
}

type DirSize struct {
	Path string
	Size string // human readable
}

type InodeSection struct {
	UsedPct float64
	Status  Status
}

type FDSection struct {
	Used    int
	Max     int
	UsedPct float64
	Status  Status
}

type DockerContainer struct {
	Name   string
	CPUPct float64
	MemMB  float64
	MemRaw string
	Status string
}

type DockerSection struct {
	Available     bool
	Reason        string // why unavailable
	Running       int
	Stopped       int
	Unhealthy     int
	Restarting    int
	Images        int
	ImagesGB      float64
	ReclaimGB     float64
	VolumesGB     float64
	BuildCacheGB  float64
	TopCPU        []DockerContainer
	TopMem        []DockerContainer
	UnhealthyList []string
	RestartLoops  []ContainerRestart
	Status        Status
	Note          string
}

type ContainerRestart struct {
	Name         string
	State        string
	RestartCount int
	DeltaCount   int     // restarts since last cached run (0 on first run)
	RatePerMin   float64 // 0 if no cache or no delta
	UptimeSec    float64 // since last (re)start; 0 if not running
	Status       Status
	Reason       string
}

type LogsSection struct {
	JournalSize string
	BigLogDirs  []DirSize
}

type ProcessRow struct {
	PID    string
	User   string
	CPUPct float64
	MemPct float64
	RSSMB  float64
	Cmd    string
}

type ProcessesSection struct {
	TopCPU []ProcessRow
	TopMem []ProcessRow
}

type ZombieProc struct {
	PID  string
	PPID string
	Cmd  string
}

type ZombiesSection struct {
	Procs  []ZombieProc
	Status Status
}

type ExtraSystem struct {
	OOMKills24h    int
	FailedUnits    int
	FailedUnitList []string
	RebootRequired bool
	RebootReason   string
	Connections    int
}

type Decision struct {
	Overall  Status
	Headline string
	Reasons  []string
	Actions  []string
}

type DeltaItem struct {
	Label     string
	Detail    string // "↓ 12%  (4.2 GB → 3.7 GB)"
	Direction Status // OK/Info → benign change, Warn/Bad → concerning
}

type SinceLastSection struct {
	HasPrevious bool
	Elapsed     time.Duration
	Items       []DeltaItem
}
