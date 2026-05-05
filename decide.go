package main

import "fmt"

// decide computes the overall headline + actions from the collected report.
func decide(r *Report) Decision {
	var d Decision
	d.Overall = StatusOK

	bump := func(s Status) {
		if s > d.Overall && s <= StatusBad {
			d.Overall = s
		}
	}

	// Per-section evaluation.
	bump(r.System.Status)
	if r.System.Status == StatusBad {
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"CPU load %.2f on %d core(s) = %.0f%% — saturated",
			r.System.Load1, r.System.Cores, r.System.LoadPct))
		d.Actions = append(d.Actions, "consider a CPU upgrade or scale workload horizontally")
	} else if r.System.Status == StatusWarn {
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"CPU load %.0f%% of capacity (5m avg %.2f, 15m avg %.2f)",
			r.System.LoadPct, r.System.Load5, r.System.Load15))
		d.Actions = append(d.Actions, "watch the 5m/15m load trend; investigate top CPU processes")
	}

	bump(r.Memory.Status)
	if r.Memory.Status == StatusBad {
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"available RAM %.0f%% (%d MB of %d MB) — RAM exhausted",
			r.Memory.AvailPct, r.Memory.AvailableMB, r.Memory.TotalMB))
		d.Actions = append(d.Actions, "RAM upgrade recommended")
	} else if r.Memory.Status == StatusWarn {
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"available RAM %.0f%% (%d MB)", r.Memory.AvailPct, r.Memory.AvailableMB))
		d.Actions = append(d.Actions, "investigate memory hogs in top-by-mem table")
	}
	bump(r.Memory.SwapStatus)
	if r.Memory.SwapStatus >= StatusWarn && r.Memory.SwapUsedMB > 0 {
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"swap usage %d MB — system under memory pressure", r.Memory.SwapUsedMB))
		if r.Memory.SwapStatus == StatusBad {
			d.Actions = append(d.Actions, "RAM upgrade strongly recommended (heavy swapping)")
		}
	}

	bump(r.Disk.Status)
	if r.Disk.Status == StatusBad {
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"root disk %.0f%% used (%.1f / %.1f GB)",
			r.Disk.UsedPct, r.Disk.UsedGB, r.Disk.TotalGB))
		d.Actions = append(d.Actions, "clean up disk OR resize the root volume")
	} else if r.Disk.Status == StatusWarn {
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"root disk %.0f%% used", r.Disk.UsedPct))
		d.Actions = append(d.Actions, "review the disk-heavy directories below for cleanup candidates")
	}

	for _, m := range r.Disk.Mounts {
		if m.Path == "/" {
			continue
		}
		if m.Status >= StatusWarn {
			bump(m.Status)
			d.Reasons = append(d.Reasons, fmt.Sprintf(
				"mount %s at %.0f%% used", m.Path, m.UsedPct))
		}
	}

	bump(r.Inodes.Status)
	if r.Inodes.Status >= StatusWarn {
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"inode usage on / is %.0f%%", r.Inodes.UsedPct))
		d.Actions = append(d.Actions, "remove many small files (caches, mail, sessions)")
	}

	bump(r.FDs.Status)
	if r.FDs.Status >= StatusWarn {
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"file descriptors %.0f%% used (%d / %d)", r.FDs.UsedPct, r.FDs.Used, r.FDs.Max))
		d.Actions = append(d.Actions, "check for fd leaks or raise fs.file-max")
	}

	if r.Docker != nil && r.Docker.Available {
		bump(r.Docker.Status)
		if r.Docker.Unhealthy > 0 {
			d.Reasons = append(d.Reasons, fmt.Sprintf(
				"%d unhealthy docker container(s)", r.Docker.Unhealthy))
		}
		if r.Docker.ReclaimGB >= 5 {
			d.Reasons = append(d.Reasons, fmt.Sprintf(
				"docker has ~%.1f GB reclaimable", r.Docker.ReclaimGB))
			d.Actions = append(d.Actions, "review 'docker system df' (script will not prune for you)")
		}
		for _, rl := range r.Docker.RestartLoops {
			if rl.Status >= StatusBad {
				d.Reasons = append(d.Reasons, fmt.Sprintf(
					"container '%s' in restart loop — %s", rl.Name, rl.Reason))
				d.Actions = append(d.Actions, fmt.Sprintf(
					"docker logs --tail 200 %s ; check restart policy and exit code", rl.Name))
			} else if rl.Status >= StatusWarn {
				d.Reasons = append(d.Reasons, fmt.Sprintf(
					"container '%s' restarting frequently — %s", rl.Name, rl.Reason))
			}
		}
	}

	if r.Zombies.Status >= StatusWarn {
		bump(StatusWarn)
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"%d zombie process(es) present", len(r.Zombies.Procs)))
		d.Actions = append(d.Actions, "restart parent processes that fail to reap children")
	}

	if r.System2.OOMKills24h >= thresholds.OOMWarn {
		bump(StatusWarn)
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"%d OOM kill(s) in last 24h", r.System2.OOMKills24h))
		if r.Memory.AvailPct <= 30 {
			bump(StatusBad)
			d.Actions = append(d.Actions, "RAM upgrade likely needed (recent OOMs + tight memory)")
		}
	}

	if r.System2.FailedUnits >= thresholds.UnitsWarn {
		bump(StatusWarn)
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"%d failed systemd unit(s)", r.System2.FailedUnits))
		d.Actions = append(d.Actions, "systemctl --failed; systemctl status <unit>")
	}

	if r.System2.RebootRequired {
		// Info only — does not bump severity.
		d.Reasons = append(d.Reasons, "reboot required: "+r.System2.RebootReason)
	}

	switch d.Overall {
	case StatusOK:
		d.Headline = "VPS resources look sufficient. Upgrade not needed."
	case StatusWarn:
		d.Headline = "Cleanup recommended."
	case StatusBad:
		d.Headline = "VPS upgrade may be needed."
	}
	return d
}
