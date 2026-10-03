package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func getFreePort(c *require.Assertions) string {
	listener, err := net.Listen("tcp", "localhost:0")
	c.NoError(err)
	defer listener.Close()

	return strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
}

func startApp(t *testing.T) string {
	t.Helper()
	c := require.New(t)

	port := getFreePort(c)
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

func get(c *require.Assertions, url string) (int, string, string) {
	response, err := http.Get(url)
	c.NoError(err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	c.NoError(err)

	return response.StatusCode, response.Header.Get("Content-Type"), string(body)
}

func TestStaticFiles(t *testing.T) {
	t.Run("open the page", func(t *testing.T) {
		c := require.New(t)
		baseURL := startApp(t)

		status, contentType, body := get(c, baseURL+"/")
		c.Equal(http.StatusOK, status)
		c.True(strings.HasPrefix(contentType, "text/html"))
		c.Contains(body, "<title>Vibranium Exchange</title>")
	})

	t.Run("open the page from the repository root", func(t *testing.T) {
		c := require.New(t)
		t.Chdir("../..")
		baseURL := startApp(t)

		status, _, body := get(c, baseURL+"/")
		c.Equal(http.StatusOK, status)
		c.Contains(body, "<title>Vibranium Exchange</title>")
	})

	t.Run("stylesheet is served", func(t *testing.T) {
		c := require.New(t)
		baseURL := startApp(t)

		status, contentType, _ := get(c, baseURL+"/app.css")
		c.Equal(http.StatusOK, status)
		c.True(strings.HasPrefix(contentType, "text/css"))
	})
}
