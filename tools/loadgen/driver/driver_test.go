package driver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/tools/loadgen/pace"
	"github.com/rdzpedraos/order-book/tools/loadgen/workload"
)

// A fake API: it answers deposits, orders and cancellations like the real one
// and counts them.
type fakeAPI struct {
	lock        sync.Mutex
	status      int
	orderStatus int
	deposits    int
	orders      []map[string]any
	closes      int
	hosts       map[string]bool
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.hosts[r.Host] = true

	if f.status != http.StatusCreated {
		w.WriteHeader(f.status)

		return
	}

	switch {
	case r.URL.Path == "/wallet/deposits":
		f.deposits++
	case r.URL.Path == "/orders" && f.orderStatus != 0:
		w.WriteHeader(f.orderStatus)

		return
	case r.URL.Path == "/orders":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["userId"] = r.Header.Get("X-User-ID")
		f.orders = append(f.orders, body)
	case strings.HasSuffix(r.URL.Path, "/close"):
		f.closes++
	}

	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"data":{"orderId":"` + uuid.NewString() + `"}}`))
}

func startAPI(t *testing.T, status int) (*fakeAPI, string) {
	t.Helper()

	api := &fakeAPI{status: status, hosts: map[string]bool{}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	return api, server.URL
}

func newConfig(c *require.Assertions, apiURL, profile string) Config {
	generator, err := workload.NewGenerator(profile, 5, 1)
	c.NoError(err)

	return Config{
		OrdersURL: apiURL, WalletURL: apiURL, Book: "BRL-VIB", People: workload.ListPeople(5), Workers: 4,
		Duration: 300 * time.Millisecond, Schedule: pace.NewConstant(200), Generator: generator,
	}
}

func TestRun(t *testing.T) {
	t.Run("people are funded and orders are sent at the rate", func(t *testing.T) {
		c := require.New(t)
		api, url := startAPI(t, http.StatusCreated)

		report, err := Run(t.Context(), newConfig(c, url, "B"), NewTracker(time.Now()))
		c.NoError(err)

		c.Equal(10, api.deposits, "each person gets BRL and VIB")
		c.InDelta(60, report.Sent, 2)
		c.Equal(report.Sent, report.Accepted)
		c.Zero(report.Failed)
		c.Equal(report.Accepted, report.Acceptance.Count)
		c.Equal(report.Accepted, report.MissingEvents, "no engine reads this fake API")
	})

	t.Run("amounts travel as decimal strings of the book's currencies", func(t *testing.T) {
		c := require.New(t)
		api, url := startAPI(t, http.StatusCreated)

		_, err := Run(t.Context(), newConfig(c, url, "A"), NewTracker(time.Now()))
		c.NoError(err)

		c.Equal(map[string]any{"book": "BRL-VIB", "side": "BUY", "limit": "100.00", "quantity": "1", "userId": "loadgen-0"}, api.orders[0])
	})

	t.Run("profile C cancels the orders it created", func(t *testing.T) {
		c := require.New(t)
		api, url := startAPI(t, http.StatusCreated)

		_, err := Run(t.Context(), newConfig(c, url, "C"), NewTracker(time.Now()))
		c.NoError(err)

		c.Positive(api.closes)
	})

	t.Run("requests go to the ingress host when one is set", func(t *testing.T) {
		c := require.New(t)
		api, url := startAPI(t, http.StatusCreated)
		config := newConfig(c, url, "B")
		config.Host = "orderbook.local"

		_, err := Run(t.Context(), config, NewTracker(time.Now()))
		c.NoError(err)

		c.Equal(map[string]bool{"orderbook.local": true}, api.hosts)
	})

	t.Run("an order the API refuses is counted as failed", func(t *testing.T) {
		c := require.New(t)
		api, url := startAPI(t, http.StatusCreated)
		api.orderStatus = http.StatusServiceUnavailable

		report, err := Run(t.Context(), newConfig(c, url, "B"), NewTracker(time.Now()))
		c.NoError(err)

		c.Positive(report.Failed)
		c.Equal(report.Sent, report.Failed)
		c.Zero(report.Acceptance.Count)
	})

	t.Run("a deposit the API refuses stops the run", func(t *testing.T) {
		c := require.New(t)
		_, url := startAPI(t, http.StatusServiceUnavailable)

		_, err := Run(t.Context(), newConfig(c, url, "B"), NewTracker(time.Now()))
		c.ErrorContains(err, "fund loadgen-")
	})

	t.Run("an unknown book", func(t *testing.T) {
		c := require.New(t)
		_, url := startAPI(t, http.StatusCreated)
		config := newConfig(c, url, "B")
		config.Book = "BTC-USD"

		_, err := Run(t.Context(), config, NewTracker(time.Now()))
		c.Error(err)
	})
}
