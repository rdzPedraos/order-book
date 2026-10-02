package applycommands

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/container"
	"gofr.dev/pkg/gofr/logging"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
)

const benchmarkCommands = 10_000

// The engine's batch size in main.go.
const benchmarkBatch = 500

// The wallet and the log are the in-memory mocks, so the benchmark measures
// the engine's own work: applying the commands to the book and building the
// events and fund operations of each batch.
func benchmarkProfile(b *testing.B, mix commandMix) {
	commands := buildCommands(require.New(b), 7, benchmarkCommands, mix)
	ctx := &gofr.Context{Context: context.Background(), Container: &container.Container{Logger: logging.NewLogger(logging.FATAL)}}

	b.ReportAllocs()

	for b.Loop() {
		b.StopTimer()
		orderbook.InitEmpty(b)
		fundTraders(walletclient.InitMock(b))
		producer.InitMock(b)
		b.StartTimer()

		for start := 0; start < len(commands); start += benchmarkBatch {
			if err := Handle(ctx, toBatch(commands[start:min(start+benchmarkBatch, len(commands))]...)); err != nil {
				b.Fatal(err)
			}
		}
	}

	b.ReportMetric(float64(benchmarkCommands*b.N)/b.Elapsed().Seconds(), "commands/s")
}

func BenchmarkHandle(b *testing.B) {
	b.Run("A everything crosses", func(b *testing.B) {
		benchmarkProfile(b, commandMix{spread: 0})
	})

	b.Run("B 90 percent rests", func(b *testing.B) {
		benchmarkProfile(b, commandMix{markets: 10, spread: 500, separated: true})
	})

	b.Run("C 50 percent cancellations", func(b *testing.B) {
		benchmarkProfile(b, commandMix{cancels: 50, spread: 100})
	})
}
