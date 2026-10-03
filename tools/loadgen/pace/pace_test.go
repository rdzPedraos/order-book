package pace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConstant(t *testing.T) {
	t.Run("a fixed rate", func(t *testing.T) {
		c := require.New(t)
		schedule := NewConstant(1000)

		c.Equal(0, schedule.CountDue(0))
		c.Equal(500, schedule.CountDue(500*time.Millisecond))
		c.Equal(1000, schedule.CountDue(time.Second))
		c.Equal(2500, schedule.CountDue(2500*time.Millisecond))
	})
}

func TestBurst(t *testing.T) {
	t.Run("the base rate, then the peak, then the base again, a third of the run each", func(t *testing.T) {
		c := require.New(t)
		schedule := NewBurst(1000, 10000, 30*time.Second)

		c.Equal(10_000, schedule.CountDue(10*time.Second))
		c.Equal(60_000, schedule.CountDue(15*time.Second))
		c.Equal(110_000, schedule.CountDue(20*time.Second))
		c.Equal(120_000, schedule.CountDue(30*time.Second))
	})

	t.Run("its rate at each moment", func(t *testing.T) {
		c := require.New(t)
		schedule := NewBurst(1000, 10000, 30*time.Second)

		c.Equal(1000, schedule.GetRate(5*time.Second))
		c.Equal(10000, schedule.GetRate(15*time.Second))
		c.Equal(1000, schedule.GetRate(25*time.Second))
	})
}
