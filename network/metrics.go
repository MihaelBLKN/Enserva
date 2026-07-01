package network

import "time"

// RuntimeMetrics contains cumulative low-overhead runtime timing counters.
type RuntimeMetrics struct {
	TicksAdvanced         uint64  `json:"ticksAdvanced"`
	LastTickDurationNs    int64   `json:"lastTickDurationNs"`
	LastTickDurationMs    float64 `json:"lastTickDurationMs"`
	MaxTickDurationNs     int64   `json:"maxTickDurationNs"`
	MaxTickDurationMs     float64 `json:"maxTickDurationMs"`
	TotalTickDurationNs   int64   `json:"totalTickDurationNs"`
	TotalTickDurationMs   float64 `json:"totalTickDurationMs"`
	AverageTickDurationMs float64 `json:"averageTickDurationMs"`
}

func durationMillis(nanoseconds int64) float64 {
	return float64(time.Duration(nanoseconds)) / float64(time.Millisecond)
}
