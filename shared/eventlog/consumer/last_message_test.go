package consumer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetLastMessage(t *testing.T) {
	t.Run("empty partition has no last message", func(t *testing.T) {
		c := require.New(t)
		topic := createTopic(t)

		last, err := GetLastMessage(context.Background(), []string{brokers}, topic, 0)
		c.NoError(err)
		c.Nil(last)
	})

	t.Run("the last message of the partition, with its offset", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		publish(c, topic, "OrderAccepted")
		message := publish(c, topic, "OrderCancelled")

		last, err := GetLastMessage(context.Background(), []string{brokers}, topic, 0)
		c.NoError(err)
		c.Equal(int64(1), last.Offset)
		c.Equal(message.ID, last.Message.ID)
	})

	t.Run("a last record that is not a message", func(t *testing.T) {
		c := require.New(t)
		topic := createTopic(t)
		publishRaw(c, topic, []byte("not a message"))

		_, err := GetLastMessage(context.Background(), []string{brokers}, topic, 0)
		c.ErrorContains(err, "decode")
	})

	t.Run("unreachable log", func(t *testing.T) {
		c := require.New(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := GetLastMessage(ctx, []string{"localhost:1"}, "any-topic", 0)
		c.Error(err)
	})
}
