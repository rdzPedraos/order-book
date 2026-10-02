package consumer

import (
	"context"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// C is the caller's context (a *gofr.Context, for instance), handed back to
// the handler as it is so this package does not import Gofr.
type Subscription[C context.Context] struct {
	route  string
	handle func(ctx C, message events.Message) error
}

// The messages published to route reach handle.
func Subscribe[C context.Context](route string, handle func(C, events.Message) error) Subscription[C] {
	return Subscription[C]{route: route, handle: handle}
}

func getTopics[C context.Context](subscriptions []Subscription[C]) ([]string, error) {
	topics := make([]string, 0, len(subscriptions))

	for _, subscription := range subscriptions {
		topic, err := events.GetTopic(subscription.route)
		if err != nil {
			return nil, err
		}

		topics = append(topics, topic)
	}

	return topics, nil
}

func findSubscription[C context.Context](subscriptions []Subscription[C], message events.Message) (Subscription[C], bool) {
	for _, subscription := range subscriptions {
		if subscription.route == message.Route {
			return subscription, true
		}
	}

	return Subscription[C]{}, false
}
