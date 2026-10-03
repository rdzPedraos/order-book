package producer

import (
	"context"
	"testing"

	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// FailTimes fails that many calls, as an unreachable log, before publishing.
type Mock struct {
	Messages  []events.Message
	Err       error
	FailTimes int
}

// Replaces the log with an in-memory one until the test ends.
func InitMock(t testing.TB) *Mock {
	mock := &Mock{}
	previous := activePublisher
	activePublisher = mock

	t.Cleanup(func() { activePublisher = previous })

	return mock
}

func (m *Mock) publish(_ context.Context, _ string, _ books.Book, message events.Message) error {
	if m.Err != nil {
		return m.Err
	}

	if m.FailTimes > 0 {
		m.FailTimes--

		return ErrNotConnected
	}

	m.Messages = append(m.Messages, message)

	return nil
}

func (m *Mock) publishBatch(ctx context.Context, batch []routedMessage) (int, error) {
	if m.Err != nil {
		return 0, m.Err
	}

	if m.FailTimes > 0 {
		m.FailTimes--

		return 0, ErrNotConnected
	}

	for _, routed := range batch {
		m.Messages = append(m.Messages, routed.message)
	}

	return len(batch), nil
}

func (m *Mock) close() {}
