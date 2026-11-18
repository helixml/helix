package types

// SessionResourceSample is one reading of a session desktop container's
// cgroup v2 counters, reported by the sandbox heartbeat. Counters are
// cumulative since the container started; the API derives rates from
// consecutive samples. Limits are 0 when unlimited.
type SessionResourceSample struct {
	SessionID string `json:"session_id" parquet:"session_id,dict"`
	// Unix milliseconds, set by the API when it receives the heartbeat.
	Timestamp int64 `json:"timestamp" parquet:"timestamp"`

	CPUUsageUsec        uint64 `json:"cpu_usage_usec" parquet:"cpu_usage_usec"`
	CPULimitMillicores  uint64 `json:"cpu_limit_millicores" parquet:"cpu_limit_millicores"`
	CPUPeriods          uint64 `json:"cpu_periods" parquet:"cpu_periods"`
	CPUThrottledPeriods uint64 `json:"cpu_throttled_periods" parquet:"cpu_throttled_periods"`
	CPUThrottledUsec    uint64 `json:"cpu_throttled_usec" parquet:"cpu_throttled_usec"`

	MemoryBytes      uint64 `json:"memory_bytes" parquet:"memory_bytes"`
	MemoryPeakBytes  uint64 `json:"memory_peak_bytes" parquet:"memory_peak_bytes"`
	MemoryLimitBytes uint64 `json:"memory_limit_bytes" parquet:"memory_limit_bytes"`
	MemoryAnonBytes  uint64 `json:"memory_anon_bytes" parquet:"memory_anon_bytes"`
	MemoryFileBytes  uint64 `json:"memory_file_bytes" parquet:"memory_file_bytes"`
	MemoryHighEvents uint64 `json:"memory_high_events" parquet:"memory_high_events"`
	MemoryMaxEvents  uint64 `json:"memory_max_events" parquet:"memory_max_events"`
	OOMKills         uint64 `json:"oom_kills" parquet:"oom_kills"`

	// I/O issued by the container, counted on top-of-stack block devices only.
	IOReadBytes  uint64 `json:"io_read_bytes" parquet:"io_read_bytes"`
	IOWriteBytes uint64 `json:"io_write_bytes" parquet:"io_write_bytes"`
	IOReadOps    uint64 `json:"io_read_ops" parquet:"io_read_ops"`
	IOWriteOps   uint64 `json:"io_write_ops" parquet:"io_write_ops"`

	// Pressure stall information: total microseconds in which some task of
	// the container was stalled on CPU, memory or I/O.
	CPUPressureUsec    uint64 `json:"cpu_pressure_usec" parquet:"cpu_pressure_usec"`
	MemoryPressureUsec uint64 `json:"memory_pressure_usec" parquet:"memory_pressure_usec"`
	IOPressureUsec     uint64 `json:"io_pressure_usec" parquet:"io_pressure_usec"`

	Pids uint64 `json:"pids" parquet:"pids"`

	Filesystems []SessionFilesystemSample `json:"filesystems,omitempty" parquet:"filesystems,list"`
}

// SessionFilesystemSample is the space on one filesystem behind the
// container's mounts. Dedicated filesystems (a per-session ZFS volume or
// quota disk) measure the session itself; shared ones measure the host volume
// the session's directories live on.
type SessionFilesystemSample struct {
	// Container paths mounted from this filesystem, comma separated.
	Paths      string `json:"paths" parquet:"paths"`
	Dedicated  bool   `json:"dedicated" parquet:"dedicated"`
	SizeBytes  uint64 `json:"size_bytes" parquet:"size_bytes"`
	UsedBytes  uint64 `json:"used_bytes" parquet:"used_bytes"`
	Inodes     uint64 `json:"inodes" parquet:"inodes"`
	InodesUsed uint64 `json:"inodes_used" parquet:"inodes_used"`
}

// SessionResources is a session's sandbox utilization over a time window.
type SessionResources struct {
	SessionID   string                    `json:"session_id"`
	Summary     SessionResourceSummary    `json:"summary"`
	Points      []SessionResourcePoint    `json:"points"`
	Filesystems []SessionFilesystemSample `json:"filesystems"`
}

// SessionResourceSummary condenses a window for sizing decisions. Percentages
// are shares of wall-clock time in the window.
type SessionResourceSummary struct {
	From              int64   `json:"from"`
	To                int64   `json:"to"`
	Samples           int     `json:"samples"`
	CPULimitCores     float64 `json:"cpu_limit_cores"`
	CPUCoresP50       float64 `json:"cpu_cores_p50"`
	CPUCoresP95       float64 `json:"cpu_cores_p95"`
	CPUCoresMax       float64 `json:"cpu_cores_max"`
	CPUThrottledPct   float64 `json:"cpu_throttled_pct"`
	MemoryLimitBytes  uint64  `json:"memory_limit_bytes"`
	MemoryMaxBytes    uint64  `json:"memory_max_bytes"`
	MemoryPeakBytes   uint64  `json:"memory_peak_bytes"`
	CPUPressurePct    float64 `json:"cpu_pressure_pct"`
	MemoryPressurePct float64 `json:"memory_pressure_pct"`
	IOPressurePct     float64 `json:"io_pressure_pct"`
	IOReadBytes       uint64  `json:"io_read_bytes"`
	IOWriteBytes      uint64  `json:"io_write_bytes"`
	MemoryHighEvents  uint64  `json:"memory_high_events"`
	OOMKills          uint64  `json:"oom_kills"`
}

// SessionResourcePoint covers the interval ending at Timestamp (Unix ms).
// Rates and percentages are averages over the interval; gauges are maxima.
type SessionResourcePoint struct {
	Timestamp         int64   `json:"timestamp"`
	CPUCores          float64 `json:"cpu_cores"`
	CPUThrottledPct   float64 `json:"cpu_throttled_pct"`
	MemoryBytes       uint64  `json:"memory_bytes"`
	CPUPressurePct    float64 `json:"cpu_pressure_pct"`
	MemoryPressurePct float64 `json:"memory_pressure_pct"`
	IOPressurePct     float64 `json:"io_pressure_pct"`
	IOReadBps         float64 `json:"io_read_bps"`
	IOWriteBps        float64 `json:"io_write_bps"`
	DiskUsedBytes     uint64  `json:"disk_used_bytes"`
	OOMKills          uint64  `json:"oom_kills"`
}
