package main

import (
	"cex-price-diff-notifications/adapters"
	"cex-price-diff-notifications/arbitrage"
	"cex-price-diff-notifications/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	rabbitMQQueueName = "arbitrage_event"
)

func main() {
	// Load .env file. It's not an error if it doesn't exist.
	_ = godotenv.Load()

	// set up a new colorful handler
	handler := tint.NewHandler(os.Stdout, &tint.Options{
		AddSource:  true,
		Level:      slog.LevelDebug,
		TimeFormat: time.Kitchen,
	})

	// create a new logger
	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Info("Application starting, initializing adapters...")

	// Create adapter instances
	binanceAdapter := adapters.NewBinanceAdapter()
	mexcAdapter, err := adapters.NewMexcAdapter()
	if err != nil {
		slog.Error("Failed to initialize Mexc adapter", "error", err)
		os.Exit(1) // Exit if a critical component fails to start
	}
	defer mexcAdapter.Close() // Ensure connections are closed on exit

	// Set up RabbitMQ
	rabbitUser := os.Getenv("RABBITMQ_DEFAULT_USER")
	rabbitPass := os.Getenv("RABBITMQ_DEFAULT_PASS")
	rabbitHost := os.Getenv("RABBITMQ_HOST")
	if rabbitHost == "" {
		rabbitHost = "rabbitmq" // Default to localhost if not set
	}
	rabbitMQURL := fmt.Sprintf("amqp://%s:%s@%s:5672/", rabbitUser, rabbitPass, rabbitHost)
	slog.Info("Connecting to RabbitMQ", "url", rabbitMQURL)

	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		slog.Error("Failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		slog.Error("Failed to open a RabbitMQ channel", "error", err)
		os.Exit(1)
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		rabbitMQQueueName, // name
		false,             // durable
		false,             // delete when unused
		false,             // exclusive
		false,             // no-wait
		nil,               // arguments
	)
	if err != nil {
		slog.Error("Failed to declare a RabbitMQ queue", "error", err)
		os.Exit(1)
	}
	slog.Info("RabbitMQ queue declared", "queue_name", q.Name)

	// Set up a channel to listen for OS signals (like Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Goroutine to handle graceful shutdown
	go func() {
		<-sigChan
		slog.Info("Shutdown signal received, closing connections...")
		mexcAdapter.Close()
		ch.Close()
		conn.Close()
		os.Exit(0)
	}()

	// Goroutine to restart Mexc adapter every 5 minutes
	go func() {
		restartTicker := time.NewTicker(5 * time.Minute)
		defer restartTicker.Stop()
		for range restartTicker.C {
			if err := mexcAdapter.Restart(); err != nil {
				slog.Error("Failed to restart Mexc adapter", "error", err)
			}
		}
	}()

	slog.Info("Adapters initialized, starting main loop.")

	// Create a ticker that fires every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		slog.Info("Fetching data...")

		allTickers := make(map[string]map[string]shared.TickerBidAsk)
		var mu sync.Mutex
		var wg sync.WaitGroup

		// Fetch Binance tickers
		wg.Add(1)
		go func() {
			defer wg.Done()
			binanceTickersDto, duration, err := binanceAdapter.GetTickers()
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
			mexcTickersDto, duration, err := mexcAdapter.GetTickers()
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

		// Update Binance funding rates
		wg.Add(1)
		go func() {
			defer wg.Done()
			duration, err := binanceAdapter.UpdateFundingRates()
			if err != nil {
				slog.Error("Failed to update Binance funding rates", "error", err)
				return
			}
			slog.Info("Binance funding rates updated", "duration", duration)
		}()

		wg.Wait()

		// Calculate and log arbitrage opportunities
		slog.Info("Calculating arbitrage opportunities...")
		spreads := arbitrage.CalculateSpreads(allTickers)

		if len(spreads) == 0 {
			slog.Info("No arbitrage opportunities found in this cycle.")
		} else {
			slog.Info("Top arbitrage opportunities found:")
			for i, s := range spreads {
				if i < 5 { // Log top 5
					slog.Info("Opportunity",
						"symbol", s.UnifiedSymbol,
						"buy_at", s.ExchangeLong,
						"sell_at", s.ExchangeShort,
						"entry_spread_%", s.EntrySpread,
						"exit_spread_%", s.ExitSpread,
					)
				}

				// Publish to RabbitMQ
				body, err := json.Marshal(s)
				if err != nil {
					slog.Error("Failed to marshal spread to JSON", "error", err)
					continue
				}

				err = ch.PublishWithContext(context.Background(),
					"",     // exchange
					q.Name, // routing key
					false,  // mandatory
					false,  // immediate
					amqp.Publishing{
						ContentType: "application/json",
						Body:        body,
					})
				if err != nil {
					slog.Error("Failed to publish a message to RabbitMQ", "error", err)
				}
			}
			slog.Info("Published arbitrage opportunities to RabbitMQ", "count", len(spreads))
		}

		slog.Info("Ticker fetching cycle complete.")
	}
}
