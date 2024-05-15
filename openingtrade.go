package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

const defaultNumberOfSymbols = 13000 // approximate number of symbols on SIP feed

func contains[T comparable](slice []T, value T) bool {
	for _, element := range slice {
		if element == value {
			return true
		}
	}
	return false
}

func tradeHandler(logger *zap.Logger, ch chan string) func(stream.Trade) {
	tradeDay := time.Time{}
	trades := make(map[string]struct{}, defaultNumberOfSymbols)

	return func(trade stream.Trade) {
		if !contains(trade.Conditions, "O") {
			return
		}

		t := trade.Timestamp.Truncate(24 * time.Hour)
		if tradeDay.Before(t) {
			logger.Info("Starting new day")
			tradeDay = t
			trades = make(map[string]struct{}, defaultNumberOfSymbols)
		}

		if _, ok := trades[trade.Symbol]; !ok {
			trades[trade.Symbol] = struct{}{}
			logger.Info("opening trade",
				zap.Int64("id", trade.ID),
				zap.String("symbol", trade.Symbol),
				zap.String("trade_time", trade.Timestamp.Format(time.RFC3339Nano)),
				zap.Strings("conditions", trade.Conditions),
				zap.Float64("price", trade.Price),
				zap.Uint32("size", trade.Size),
				zap.String("exchange", trade.Exchange),
				zap.String("tape", trade.Tape),
			)
			ch <- trade.Symbol
		}
	}
}

func main() {
	logConfig := zap.NewProductionConfig()
	logConfig.Sampling = nil
	logger, err := logConfig.Build()
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-s
		cancel()
	}()

	if len(os.Args) != 2 {
		logger.Fatal("invalid number of arguments")
	}

	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		logger.Fatal("failed to read file", zap.String("file", os.Args[1]), zap.Error(err))
	}

	symbols := []string{}
	if err := yaml.Unmarshal(b, &symbols); err != nil {
		logger.Fatal("failed to unmarshal file", zap.String("file", os.Args[1]), zap.Error(err))
	}

	c := stream.NewStocksClient(
		marketdata.SIP,
		stream.WithLogger(logger.Sugar()),
	)
	if err := c.Connect(ctx); err != nil {
		logger.Fatal("could not establish connection", zap.Error(err))
	}
	logger.Info("connection established")

	length := len(symbols)
	if contains(symbols, "*") {
		length = defaultNumberOfSymbols
	}
	unsubscribeCh := make(chan string, length)
	if err := c.SubscribeToTrades(tradeHandler(logger, unsubscribeCh), symbols...); err != nil {
		logger.Fatal("subscription problem", zap.Error(err))
	}
	logger.Info("symbols subscribed", zap.Int("symbols", len(symbols)))

	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case symbol := <-unsubscribeCh:
				if err := c.UnsubscribeFromTrades(symbol); err != nil {
					logger.Error("unsubscribe", zap.Error(err))
				}
			}
		}
	}(ctx)

	if err := <-c.Terminated(); err != nil {
		logger.Fatal("terminated with error", zap.Error(err))
	}
}
