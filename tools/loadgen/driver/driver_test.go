package driver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/tools/loadgen/pace"
	"github.com/rdzpedraos/order-book/tools/loadgen/stats"
	"github.com/rdzpedraos/order-book/tools/loadgen/workload"
)

// A fake API: it answers deposits and orders like the real one and counts them.
type fakeAPI struct {
	lock        sync.Mutex
	status      int
	orderStatus int
	deposits    int
	orders      []map[string]any
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.lock.Lock()
	defer f.lock.Unlock()

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
	}

	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"data":{"orderId":"` + uuid.NewString() + `"}}`))
}

func startAPI(t *testing.T, status int) (*fakeAPI, string) {
	t.Helper()

	api := &fakeAPI{status: status}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	return api, server.URL
}

func newConfig(apiURL string) Config {
	return Config{
		OrdersURL: apiURL, WalletURL: apiURL, Book: "BRL-VIB", People: workload.ListPeople(5), Workers: 4,
		Duration: 300 * time.Millisecond, Schedule: pace.NewConstant(200), Generator: workload.NewGenerator(5, 1),
		Progress: io.Discard, ProgressEvery: time.Hour,
	}
}

func TestRun(t *testing.T) {
	t.Run("people are funded and orders are sent at the rate", func(t *testing.T) {
		c := require.New(t)
		api, url := startAPI(t, http.StatusCreated)

		report, err := Run(t.Context(), newConfig(url), NewTracker(time.Now()))
		c.NoError(err)

		c.Equal(10, api.deposits, "each person gets BRL and VIB")
		c.InDelta(60, report.Sent, 2)
		c.Equal(report.Sent, report.Accepted)
		c.Zero(report.Failed)
		c.Equal(report.Accepted, report.Acceptance.Count)
		c.Equal(report.Accepted, report.MissingEvents, "no engine reads this fake API")
		c.Zero(report.EngineRate)
	})

	t.Run("amounts travel as decimal strings of the book's currencies", func(t *testing.T) {
		c := require.New(t)
		api, url := startAPI(t, http.StatusCreated)

		_, err := Run(t.Context(), newConfig(url), NewTracker(time.Now()))
		c.NoError(err)

		marketBuys := 0
		for _, order := range api.orders {
			c.Equal("BRL-VIB", order["book"])

			if amount, isMarketBuy := order["amount"]; isMarketBuy {
				c.Equal("105.00", amount)
				marketBuys++

				continue
			}

			c.Regexp(`^\d+$`, order["quantity"])
			if limit, isLimit := order["limit"]; isLimit {
				c.Regexp(`^\d+\.\d{2}$`, limit)
			}
		}
		c.Positive(marketBuys)
	})

	t.Run("the run writes its phases and a progress line every interval", func(t *testing.T) {
		c := require.New(t)
		_, url := startAPI(t, http.StatusCreated)
		var progress bytes.Buffer
		config := newConfig(url)
		config.Progress, config.ProgressEvery = &progress, 50*time.Millisecond

		_, err := Run(t.Context(), config, NewTracker(time.Now()))
		c.NoError(err)

		c.Contains(progress.String(), "funding 5 people\nsending for 300ms\n")
		c.Regexp(`(?m)^ +\d+s  API +\d+/s  engine +0/s  backlog \d+$`, progress.String())
		c.Contains(progress.String(), "waiting up to 0s for the last engine events\n")
	})

	t.Run("an order the API refuses is counted as failed", func(t *testing.T) {
		c := require.New(t)
		api, url := startAPI(t, http.StatusCreated)
		api.orderStatus = http.StatusServiceUnavailable

		report, err := Run(t.Context(), newConfig(url), NewTracker(time.Now()))
		c.NoError(err)

		c.Positive(report.Failed)
		c.Equal(report.Sent, report.Failed)
		c.Zero(report.Acceptance.Count)
	})

	t.Run("a deposit the API refuses stops the run", func(t *testing.T) {
		c := require.New(t)
		_, url := startAPI(t, http.StatusServiceUnavailable)

		_, err := Run(t.Context(), newConfig(url), NewTracker(time.Now()))
		c.ErrorContains(err, "fund loadgen-")
	})

	t.Run("an unknown book", func(t *testing.T) {
		c := require.New(t)
		_, url := startAPI(t, http.StatusCreated)
		config := newConfig(url)
		config.Book = "BTC-USD"

		_, err := Run(t.Context(), config, NewTracker(time.Now()))
		c.Error(err)
	})
}

func TestIsEngineKeepingUp(t *testing.T) {
	t.Run("every order has its event and the p99 is under a second", func(t *testing.T) {
		c := require.New(t)

		c.True(Report{Engine: stats.Summary{Count: 10, P99: 900 * time.Millisecond}}.IsEngineKeepingUp())
	})

	t.Run("an order without its event falls behind", func(t *testing.T) {
		c := require.New(t)

		c.False(Report{Engine: stats.Summary{Count: 10, P99: time.Millisecond}, MissingEvents: 1}.IsEngineKeepingUp())
	})

	t.Run("a p99 of a second or more falls behind", func(t *testing.T) {
		c := require.New(t)

		c.False(Report{Engine: stats.Summary{Count: 10, P99: time.Second}}.IsEngineKeepingUp())
	})

	t.Run("a run without events does not keep up", func(t *testing.T) {
		c := require.New(t)

		c.False(Report{}.IsEngineKeepingUp())
	})
}
