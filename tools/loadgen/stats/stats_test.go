package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSummarize(t *testing.T) {
	t.Run("percentiles of a thousand samples", func(t *testing.T) {
		c := require.New(t)
		samples := make([]time.Duration, 0, 1000)
		for i := 1000; i >= 1; i-- {
			samples = append(samples, time.Duration(i)*time.Millisecond)
		}

		summary := Summarize(samples)

		c.Equal(1000, summary.Count)
		c.Equal(500*time.Millisecond, summary.P50)
		c.Equal(900*time.Millisecond, summary.P90)
		c.Equal(990*time.Millisecond, summary.P99)
		c.Equal(999*time.Millisecond, summary.P999)
		c.Equal(1000*time.Millisecond, summary.Max)
	})

	t.Run("one sample is every percentile", func(t *testing.T) {
		c := require.New(t)

		summary := Summarize([]time.Duration{7 * time.Millisecond})

		c.Equal(7*time.Millisecond, summary.P50)
		c.Equal(7*time.Millisecond, summary.P999)
	})

	t.Run("no samples", func(t *testing.T) {
		c := require.New(t)

		c.Equal(Summary{}, Summarize(nil))
	})
}
