package applycommands

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/money"
)

const traders = 20

// The share of each kind of command, in percent; the rest are limit orders.
type commandMix struct {
	cancels  int
	modifies int
	markets  int
	// How far from the mid price a limit order goes, in cents: a narrow
	// spread crosses often, a wide one mostly rests.
	spread int64
	// Buys only below the mid price and sells only above, so limit orders
	// never cross and only market orders trade.
	separated bool
}

func getTraderID(index int) string {
	return "trader-" + strconv.Itoa(index)
}

func fundTraders(wallet *walletclient.Mock) {
	for index := range traders {
		wallet.Deposit(getTraderID(index), money.BRL, 1_000_000_000_000)
		wallet.Deposit(getTraderID(index), money.VIB, 1_000_000_000)
	}
}

// Commands of random people, sides, prices and quantities around R$ 100.00,
// the same ones for the same seed.
func buildCommands(c *require.Assertions, seed uint64, count int, mix commandMix) []events.Message {
	random := rand.New(rand.NewPCG(seed, seed))
	commands := make([]events.Message, 0, count)
	placed := []events.NewOrder{}

	for range count {
		roll := random.IntN(100)

		switch {
		case roll < mix.cancels && len(placed) > 0:
			order := placed[random.IntN(len(placed))]
			commands = append(commands, cancelOrder(c, order.OrderID, order.UserID))
		case roll < mix.cancels+mix.modifies && len(placed) > 0:
			order := placed[random.IntN(len(placed))]
			commands = append(commands, modifyOrder(c, order.OrderID, order.UserID, ptr(10000+random.Int64N(2*mix.spread+1)-mix.spread), nil))
		default:
			order := buildRandomOrder(random, roll < mix.cancels+mix.modifies+mix.markets, mix)
			placed = append(placed, order)
			commands = append(commands, newOrder(c, order))
		}
	}

	return commands
}

func buildRandomOrder(random *rand.Rand, isMarket bool, mix commandMix) events.NewOrder {
	order := events.NewOrder{OrderID: uuid.New(), UserID: getTraderID(random.IntN(traders)), Side: "BUY", Type: "LIMIT"}
	if random.IntN(2) == 0 {
		order.Side = "SELL"
	}

	quantity := 1 + random.Int64N(10)

	if !isMarket {
		order.Limit, order.Quantity = ptr(getRandomPrice(random, order.Side, mix)), &quantity

		return order
	}

	order.Type = "MARKET"
	if order.Side == "BUY" {
		order.Amount = ptr(quantity * 10000)
	} else {
		order.Quantity = &quantity
	}

	return order
}

func getRandomPrice(random *rand.Rand, side string, mix commandMix) int64 {
	if !mix.separated {
		return 10000 + random.Int64N(2*mix.spread+1) - mix.spread
	}

	if side == "BUY" {
		return 10000 - 1 - random.Int64N(mix.spread)
	}

	return 10000 + 1 + random.Int64N(mix.spread)
}

func applyInBatches(t *testing.T, commands []events.Message) {
	t.Helper()
	c := require.New(t)

	for start := 0; start < len(commands); start += 500 {
		c.NoError(Handle(newContext(t), toBatch(commands[start:min(start+500, len(commands))]...)))
	}
}

func TestDeterminism(t *testing.T) {
	t.Run("identical replay", func(t *testing.T) {
		c := require.New(t)
		commands := buildCommands(c, 42, 10_000, commandMix{cancels: 15, modifies: 10, markets: 10, spread: 100})

		wallet, firstLog := setUp(t)
		fundTraders(wallet)
		applyInBatches(t, commands)

		wallet, secondLog := setUp(t)
		fundTraders(wallet)
		applyInBatches(t, commands)

		c.NotEmpty(getTrades(c, firstLog), "the commands must cross for the replay to mean something")
		c.Len(secondLog.Messages, len(firstLog.Messages))

		for index, message := range firstLog.Messages {
			c.Equal(message, secondLog.Messages[index], "event %d", index)
		}
	})
}
