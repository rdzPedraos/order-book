package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatus(t *testing.T) {
	t.Run("every status of the lifecycle is valid", func(t *testing.T) {
		c := require.New(t)

		c.True(IsValidStatus(StatusPending))
		c.True(IsValidStatus(StatusOpen))
		c.True(IsValidStatus(StatusPartiallyFilled))
		c.True(IsValidStatus(StatusFilled))
		c.True(IsValidStatus(StatusCancelled))
		c.True(IsValidStatus(StatusRejected))
		c.False(IsValidStatus("CLOSED"))
	})

	t.Run("filled, cancelled and rejected are final", func(t *testing.T) {
		c := require.New(t)

		c.True(StatusFilled.IsFinal())
		c.True(StatusCancelled.IsFinal())
		c.True(StatusRejected.IsFinal())
		c.False(StatusPending.IsFinal())
		c.False(StatusOpen.IsFinal())
		c.False(StatusPartiallyFilled.IsFinal())
	})
}
