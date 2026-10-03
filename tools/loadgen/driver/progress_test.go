package driver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFormatProgress(t *testing.T) {
	t.Run("the API and engine rates of one interval and the backlog", func(t *testing.T) {
		c := require.New(t)

		line := formatProgress(10*time.Second, 5*time.Second, counts{accepted: 50000, processed: 33000}, counts{accepted: 25000, processed: 16500})

		c.Equal("  10s  API   5000/s  engine   3300/s  backlog 17000", line)
	})

	t.Run("an event read before its order's answer leaves no negative backlog", func(t *testing.T) {
		c := require.New(t)

		line := formatProgress(5*time.Second, 5*time.Second, counts{accepted: 10, processed: 11}, counts{})

		c.Equal("   5s  API      2/s  engine      2/s  backlog 0", line)
	})
}
