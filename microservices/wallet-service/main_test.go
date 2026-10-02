package main

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
)

func getFreePort(c *require.Assertions) string {
	listener, err := net.Listen("tcp", "localhost:0")
	c.NoError(err)
	defer listener.Close()

	return strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
}

// Starts the app in role and answers its base URL; it stops when the test ends.
func startApp(t *testing.T, role string) string {
	t.Helper()
	c := require.New(t)

	port := getFreePort(c)
	t.Setenv("ROLE", role)
	t.Setenv("HTTP_PORT", port)
	t.Setenv("METRICS_PORT", getFreePort(c))

	app := buildApp()
	go app.Run()
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	baseURL := "http://localhost:" + port
	c.Eventually(func() bool {
		response, err := http.Get(baseURL + "/.well-known/alive")
		if err == nil {
			response.Body.Close()
		}

		return err == nil
	}, 5*time.Second, 20*time.Millisecond)

	return baseURL
}

func send(c *require.Assertions, method, url string) int {
	request, err := http.NewRequest(method, url, strings.NewReader(`{"operations":[]}`))
	c.NoError(err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-User-ID", "user-a")

	response, err := http.DefaultClient.Do(request)
	c.NoError(err)
	response.Body.Close()

	return response.StatusCode
}

func TestRoles(t *testing.T) {
	t.Run("internal route in the public role", func(t *testing.T) {
		c := require.New(t)
		baseURL := startApp(t, "api")

		c.Equal(http.StatusNotFound, send(c, http.MethodPost, baseURL+"/wallet/internal/funds:batch"))
		c.NotEqual(http.StatusNotFound, send(c, http.MethodGet, baseURL+"/wallet"))
	})

	t.Run("public route in the internal role", func(t *testing.T) {
		c := require.New(t)
		baseURL := startApp(t, "funds")

		c.Equal(http.StatusNotFound, send(c, http.MethodGet, baseURL+"/wallet"))
		c.NotEqual(http.StatusNotFound, send(c, http.MethodPost, baseURL+"/wallet/internal/funds:batch"))
	})
}

// The trades role has no routes, so it is alive once its metrics port answers.
func startTradesRole(t *testing.T) (httpPort string) {
	t.Helper()
	c := require.New(t)

	cluster, err := kfake.NewCluster(kfake.SeedTopics(1, "orders.events"))
	c.NoError(err)
	t.Cleanup(cluster.Close)

	httpPort, metricsPort := getFreePort(c), getFreePort(c)
	t.Setenv("ROLE", "trades")
	t.Setenv("HTTP_PORT", httpPort)
	t.Setenv("METRICS_PORT", metricsPort)
	t.Setenv("EVENT_LOG_BROKERS", cluster.ListenAddrs()[0])
	t.Setenv("EVENT_LOG_GROUP", "wallet-trades-test")

	app := buildApp()
	go app.Run()
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	c.Eventually(func() bool {
		response, err := http.Get("http://localhost:" + metricsPort + "/metrics")
		if err == nil {
			response.Body.Close()
		}

		return err == nil
	}, 5*time.Second, 20*time.Millisecond)

	return httpPort
}

func TestTradesRole(t *testing.T) {
	t.Run("the trades role serves no route", func(t *testing.T) {
		c := require.New(t)
		httpPort := startTradesRole(t)

		_, err := http.Get("http://localhost:" + httpPort + "/wallet")
		c.Error(err, "the trades role must not open its HTTP port")
	})
}
