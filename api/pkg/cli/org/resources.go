package org

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func newSessionResourcesCmd() *cobra.Command {
	var (
		hours   int
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "resources [session-id]",
		Short: "CPU, memory, I/O, stalls and disk use of a session's sandbox",
		Long: `Summarise how a session's desktop container used its resources, sampled every 30 seconds:
CPU cores (p50, p95, max) against the CPU limit, memory against the memory limit, time stalled
on CPU, memory and I/O (Linux pressure stall information), CPU throttling, OOM kills, disk I/O,
and space on each filesystem behind the container's mounts.

Inside a sandbox the session id defaults to $HELIX_SESSION_ID. --json prints the summary and
the chart points.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sid := os.Getenv("HELIX_SESSION_ID")
			if len(args) == 1 {
				sid = args[0]
			}
			if sid == "" {
				return errors.New("session id required (or set HELIX_SESSION_ID)")
			}
			c, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			ctx, cancel := callCtx(cmd.Context(), time.Minute)
			defer cancel()
			r, err := c.GetSessionResources(ctx, sid, hours)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(r)
			}
			printSessionResources(r)
			return nil
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 24, "Hours of history, 1 through 720")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output (summary, points and filesystems)")
	return cmd
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func limitText(limit float64, format func(float64) string) string {
	if limit == 0 {
		return "no limit"
	}
	return "limit " + format(limit)
}

func printSessionResources(r *types.SessionResources) {
	s := r.Summary
	if s.Samples == 0 {
		fmt.Println("No sandbox samples in this window.")
		return
	}
	cores := func(v float64) string { return fmt.Sprintf("%.2f", v) }
	fmt.Printf("Window:   %s → %s (%d samples)\n",
		time.UnixMilli(s.From).Format(time.DateTime), time.UnixMilli(s.To).Format(time.DateTime), s.Samples)
	fmt.Printf("CPU:      p50 %s  p95 %s  max %s cores (%s)\n",
		cores(s.CPUCoresP50), cores(s.CPUCoresP95), cores(s.CPUCoresMax), limitText(s.CPULimitCores, cores))
	fmt.Printf("Memory:   max %s, peak since start %s (%s)\n", humanBytes(s.MemoryMaxBytes), humanBytes(s.MemoryPeakBytes),
		limitText(float64(s.MemoryLimitBytes), func(v float64) string { return humanBytes(uint64(v)) }))
	fmt.Printf("Stalled:  CPU %.1f%%  memory %.1f%%  I/O %.1f%% of the time\n", s.CPUPressurePct, s.MemoryPressurePct, s.IOPressurePct)
	fmt.Printf("Throttle: CPU throttled %.1f%% of the time, %d memory.high hits, %d OOM kills\n", s.CPUThrottledPct, s.MemoryHighEvents, s.OOMKills)
	fmt.Printf("Disk I/O: %s read, %s written\n", humanBytes(s.IOReadBytes), humanBytes(s.IOWriteBytes))
	for _, f := range r.Filesystems {
		scope := "shared host volume"
		if f.Dedicated {
			scope = "this session"
		}
		inodes := "-"
		if f.Inodes > 0 {
			inodes = fmt.Sprintf("%.0f%%", 100*float64(f.InodesUsed)/float64(f.Inodes))
		}
		fmt.Printf("Disk:     %s / %s used, inodes %s — %s (%s)\n", humanBytes(f.UsedBytes), humanBytes(f.SizeBytes), inodes, f.Paths, scope)
	}
}
