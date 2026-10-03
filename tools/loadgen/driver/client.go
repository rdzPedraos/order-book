package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/money"
	"github.com/rdzpedraos/order-book/tools/loadgen/workload"
)

type client struct {
	http      *http.Client
	ordersURL string
	walletURL string
	book      books.Book
}

type orderBody struct {
	Book     string  `json:"book"`
	Side     string  `json:"side"`
	Limit    *string `json:"limit,omitempty"`
	Quantity *string `json:"quantity,omitempty"`
	Amount   *string `json:"amount,omitempty"`
}

func newClient(config Config) (*client, error) {
	book, err := books.Normalize(config.Book)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{MaxIdleConns: config.Workers, MaxIdleConnsPerHost: config.Workers}

	return &client{
		http: &http.Client{Transport: transport}, ordersURL: config.OrdersURL, walletURL: config.WalletURL, book: book,
	}, nil
}

func (c *client) postDeposit(ctx context.Context, userID string, currency money.Currency, amount string) error {
	body := map[string]string{"currency": string(currency), "amount": amount}

	_, err := c.post(ctx, c.walletURL+"/wallet/deposits", userID, body)

	return err
}

// Answers the order id the API gave the new order.
func (c *client) postOrder(ctx context.Context, request workload.Request) (uuid.UUID, error) {
	answer, err := c.post(ctx, c.ordersURL+"/orders", request.UserID, c.buildOrderBody(request))
	if err != nil {
		return uuid.Nil, err
	}

	var created struct {
		Data struct {
			OrderID uuid.UUID `json:"orderId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(answer, &created); err != nil {
		return uuid.Nil, fmt.Errorf("read the created order: %w", err)
	}

	return created.Data.OrderID, nil
}

// Prices and amounts are in the book's quote and quantities in its base.
func (c *client) buildOrderBody(request workload.Request) orderBody {
	return orderBody{
		Book: c.book.ID, Side: request.Side,
		Limit:    formatAmount(c.book.Quote, request.Limit),
		Quantity: formatAmount(c.book.Base, request.Quantity),
		Amount:   formatAmount(c.book.Quote, request.Amount),
	}
}

func formatAmount(currency money.Currency, amount *int64) *string {
	if amount == nil {
		return nil
	}

	formatted, err := money.Format(currency, *amount)
	if err != nil {
		return nil
	}

	return &formatted
}

// Only 201 is success: every endpoint the load uses answers it.
func (c *client) post(ctx context.Context, url, userID string, body any) ([]byte, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-User-ID", userID)

	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	answer, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("POST %s answered %d", url, response.StatusCode)
	}

	return answer, nil
}
