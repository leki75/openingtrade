package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

const numSymbols = 13000 // approximate number of symbols on SIP feed

func contains[T comparable](slice []T, value T) bool {
	for _, element := range slice {
		if element == value {
			return true
		}
	}
	return false
}

func tradeHandler(logger *zap.Logger) func(stream.Trade) {
	tradeDay := time.Time{}
	trades := make(map[string]struct{}, numSymbols)

	return func(trade stream.Trade) {
		if !contains(trade.Conditions, "O") {
			return
		}

		t := trade.Timestamp.Truncate(24 * time.Hour)
		if tradeDay.Before(t) {
			logger.Info("Starting new day")
			tradeDay = t
			trades = make(map[string]struct{}, numSymbols)
		}

		if _, ok := trades[trade.Symbol]; !ok {
			trades[trade.Symbol] = struct{}{}
			logger.Info("opening trade",
				zap.String("symbol", trade.Symbol),
				zap.String("trade_time", trade.Timestamp.Format(time.RFC3339Nano)),
				zap.Strings("conditions", trade.Conditions),
				zap.Float64("price", trade.Price),
				zap.Uint32("size", trade.Size),
				zap.String("tape", trade.Tape),
				zap.String("exchange", trade.Exchange),
			)
		}
	}
}

func main() {
	logger, _ := zap.NewProduction()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-s
		cancel()
	}()

	c := stream.NewStocksClient(
		marketdata.SIP,
		stream.WithTrades(tradeHandler(logger), "*"),
		stream.WithLogger(logger.Sugar()),
	)

	if err := c.Connect(ctx); err != nil {
		logger.Fatal("could not establish connection", zap.Error(err))
	}
	logger.Info("established connection")

	if err := <-c.Terminated(); err != nil {
		logger.Fatal("terminated with error", zap.Error(err))
	}
}
