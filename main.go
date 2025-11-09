package main

import (
	"cex-price-diff-notifications/adapters"
	"cex-price-diff-notifications/arbitrage"
	"cex-price-diff-notifications/shared"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/lmittmann/tint"
)

func main() {
	// set up a new colorful handler
	handler := tint.NewHandler(os.Stdout, &tint.Options{
		AddSource:  true,
		Level:      slog.LevelDebug,
		TimeFormat: time.Kitchen,
	})

	// create a new logger
	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Info("Application starting")

	// Create a ticker that fires every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop() // Ensure the ticker is stopped when main exits

	for range ticker.C {
		slog.Info("Fetching tickers...")

		// This map will hold all tickers, structured for the calculator.
		// map[unified_symbol] -> map[exchange_name] -> Ticker
		allTickers := make(map[string]map[string]shared.TickerBidAsk)
		var mu sync.Mutex // Mutex to protect concurrent writes to the map.
		var wg sync.WaitGroup

		// Fetch Binance tickers
		wg.Add(1)
		go func() {
			defer wg.Done()
			binanceTickersDto, duration, err := adapters.GetBinanceTickers()
			if err != nil {
				slog.Error("Failed to get Binance tickers", "error", err)
				return
			}
			slog.Info("Binance tickers fetched", "count", len(binanceTickersDto), "duration", duration)

			for _, dto := range binanceTickersDto {
				genericTicker, err := dto.ToTickerBidAsk()
				if err != nil {
					if !errors.Is(err, shared.ErrUnsupportedQuoteCurrency) {
						slog.Warn("Failed to convert Binance DTO", "symbol", dto.Symbol, "error", err)
					}
					continue
				}

				mu.Lock()
				if _, ok := allTickers[genericTicker.UnifiedSymbol]; !ok {
					allTickers[genericTicker.UnifiedSymbol] = make(map[string]shared.TickerBidAsk)
				}
				allTickers[genericTicker.UnifiedSymbol]["Binance"] = genericTicker
				mu.Unlock()
			}
		}()

		// Fetch Mexc tickers
		wg.Add(1)
		go func() {
			defer wg.Done()
			mexcTickersDto, duration, err := adapters.GetMexcTickers()
			if err != nil {
				slog.Error("Failed to get Mexc tickers", "error", err)
				return
			}
			slog.Info("Mexc tickers fetched", "count", len(mexcTickersDto), "duration", duration)

			for _, dto := range mexcTickersDto {
				genericTicker, err := dto.ToTickerBidAsk()
				if err != nil {
					if !errors.Is(err, shared.ErrUnsupportedQuoteCurrency) {
						slog.Warn("Failed to convert Mexc DTO", "symbol", dto.Symbol, "error", err)
					}
					continue
				}

				mu.Lock()
				if _, ok := allTickers[genericTicker.UnifiedSymbol]; !ok {
					allTickers[genericTicker.UnifiedSymbol] = make(map[string]shared.TickerBidAsk)
				}
				allTickers[genericTicker.UnifiedSymbol]["Mexc"] = genericTicker
				mu.Unlock()
			}
		}()

		wg.Wait() // Wait for both goroutines to complete

		// Calculate and log arbitrage opportunities
		slog.Info("Calculating arbitrage opportunities...")
		spreads := arbitrage.CalculateSpreads(allTickers)

		if len(spreads) == 0 {
			slog.Info("No arbitrage opportunities found in this cycle.")
		} else {
			slog.Info("Top arbitrage opportunities found:")
			for i, s := range spreads {
				if i >= 5 { // Log top 5
					break
				}
				slog.Info("Opportunity",
					"symbol", s.UnifiedSymbol,
					"buy_at", s.ExchangeLong,
					"sell_at", s.ExchangeShort,
					"entry_spread_%", s.EntrySpread,
					"exit_spread_%", s.ExitSpread, // Added exit spread
				)
			}
		}

		slog.Info("Ticker fetching cycle complete.")
	}
}
