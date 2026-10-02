// Package events is the contract of the exchange's log (Redpanda, Kafka API):
// one envelope, Message, for every message of every topic, and the catalog of
// topics, types and payloads. Publishing is eventlog/producer and reading is
// eventlog/consumer; neither knows the payloads.
//
// It is shared because every service that reads or writes the log must agree
// on every field.
package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rdzpedraos/order-book/shared/fault"
)

const SchemaVersion = 1

var ErrInvalidRoute = fault.NewWithStatus(http.StatusInternalServerError, "invalid_route", "a route is <topic>.<type>")

// A message is named by its route, "<topic>.<type>": it is published to the
// topic and a handler subscribes to the route. Every message belongs to a
// book, whose partition it goes to, so a book's messages are read in one total
// order. The ID is generated once, before
// publishing: a retried publication keeps it, so consumers drop what they
// already applied.
type Message struct {
	ID            uuid.UUID       `json:"id"`
	Route         string          `json:"route"`
	Book          string          `json:"book"`
	SchemaVersion int             `json:"schemaVersion"`
	CreatedAt     time.Time       `json:"createdAt"`
	Payload       json.RawMessage `json:"payload"`
}

func NewMessage(route, book string, createdAt time.Time, payload any) (Message, error) {
	messageID, err := uuid.NewV7()
	if err != nil {
		return Message{}, fmt.Errorf("generate message id: %w", err)
	}

	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return Message{}, fmt.Errorf("encode %s payload: %w", route, err)
	}

	return Message{
		ID:            messageID,
		Route:         route,
		Book:          book,
		SchemaVersion: SchemaVersion,
		CreatedAt:     createdAt.UTC(),
		Payload:       encodedPayload,
	}, nil
}

func (m Message) ParsePayload(target any) error {
	if err := json.Unmarshal(m.Payload, target); err != nil {
		return fmt.Errorf("decode %s payload: %w", m.Route, err)
	}

	return nil
}

// The type is what follows the last dot; the rest is the topic.
func GetTopic(route string) (string, error) {
	lastDot := strings.LastIndex(route, ".")
	if lastDot <= 0 {
		return "", fmt.Errorf("%w: %q", ErrInvalidRoute, route)
	}

	return route[:lastDot], nil
}
