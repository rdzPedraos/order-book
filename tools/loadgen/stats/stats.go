// Package stats summarizes the latencies of a load run in percentiles.
package stats

import (
	"math"
	"slices"
	"time"
)

type Summary struct {
	Count int
	P50   time.Duration
	P90   time.Duration
	P99   time.Duration
	P999  time.Duration
	Max   time.Duration
}

func Summarize(samples []time.Duration) Summary {
	if len(samples) == 0 {
		return Summary{}
	}

	sorted := slices.Clone(samples)
	slices.Sort(sorted)

	return Summary{
		Count: len(sorted),
		P50:   getPercentile(sorted, 0.50),
		P90:   getPercentile(sorted, 0.90),
		P99:   getPercentile(sorted, 0.99),
		P999:  getPercentile(sorted, 0.999),
		Max:   sorted[len(sorted)-1],
	}
}

// Nearest rank: the smallest sample that at least that share of samples does not exceed.
func getPercentile(sorted []time.Duration, share float64) time.Duration {
	rank := int(math.Ceil(share * float64(len(sorted))))

	return sorted[max(rank, 1)-1]
}
