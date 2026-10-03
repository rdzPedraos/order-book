// Command client serves the static web page of the demo: the folder static/,
// at /. It has no handlers, store or database; the page calls the public API
// (/orders, /wallet, /market) on the same host, through the ingress.
package main

import (
	"os"

	"gofr.dev/pkg/gofr"
)

// The image keeps static/ next to the binary, while a developer runs the
// service from the service folder or from the repository root.
var staticDirs = []string{"./static", "./microservices/client/static"}

func main() {
	buildApp().Run()
}

func buildApp() *gofr.App {
	app := gofr.New()

	app.AddStaticFiles("/", getStaticDir())

	return app
}

func getStaticDir() string {
	for _, dir := range staticDirs {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	return staticDirs[0]
}
