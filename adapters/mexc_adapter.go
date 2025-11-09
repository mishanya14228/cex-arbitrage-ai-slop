package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"cex-price-diff-notifications/shared"

	"github.com/go-redis/redis/v8"
)

const (
	mexcFuturesURL         = "https://contract.mexc.com"
	mexcContractDetailPath = "/api/v1/contract/detail"
	mexcTickersPath        = "/api/v1/contract/ticker"
	mexcFundingRatePath    = "/api/v1/contract/funding_rate/" // Note the trailing slash
	redisMexcFundingPrefix = "mexc:funding_rate:"
	redisTTL               = 8 * time.Hour
)

// MexcAdapter holds state and logic for interacting with the Mexc API.
type MexcAdapter struct {
	FundingRates map[string]MexcFundingRateDto
	mu           sync.RWMutex
	redisClient  *redis.Client
}

// NewMexcAdapter creates a new instance of the MexcAdapter.
func NewMexcAdapter() (*MexcAdapter, error) {
	slog.Info("Initializing Mexc adapter...")

	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "redis:6379", // Redis host and port
		Password: redisPassword,
		DB:       0, // default DB
	})

	// Ping Redis to check connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	slog.Info("Connected to Redis successfully.")

	adapter := &MexcAdapter{
		FundingRates: make(map[string]MexcFundingRateDto),
		redisClient:  redisClient,
	}

	return adapter, nil
}

// Close closes the Redis client connection.
func (a *MexcAdapter) Close() error {
	if a.redisClient != nil {
		slog.Info("Closing Redis client connection...")
		return a.redisClient.Close()
	}
	return nil
}

// LoadFundingRatesFromRedis loads Mexc funding rates from Redis into the adapter's cache.
func (a *MexcAdapter) LoadFundingRatesFromRedis() {
	a.mu.Lock()
	defer a.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	keys, err := a.redisClient.Keys(ctx, redisMexcFundingPrefix+"*").Result()
	if err != nil {
		slog.Error("Failed to get Redis keys for Mexc funding rates", "error", err)
		return
	}

	if len(keys) == 0 {
		slog.Info("No Mexc funding rates found in Redis to load.")
		return
	}

	slog.Info("Loading Mexc funding rates from Redis...", "count", len(keys))
	for _, key := range keys {
		val, err := a.redisClient.Get(ctx, key).Result()
		if err != nil {
			slog.Warn("Failed to get Mexc funding rate from Redis", "key", key, "error", err)
			continue
		}

		var dto MexcFundingRateDto
		if err := json.Unmarshal([]byte(val), &dto); err != nil {
			slog.Warn("Failed to unmarshal Mexc funding rate from Redis", "key", key, "error", err)
			continue
		}
		unifiedSymbol := strings.TrimPrefix(key, redisMexcFundingPrefix)
		a.FundingRates[unifiedSymbol] = dto
	}
	slog.Info("Finished loading Mexc funding rates from Redis.", "loaded_count", len(a.FundingRates))
}

// UpdateFundingRates fetches funding rates for all symbols from Mexc using a rate-limited HTTP approach.
func (a *MexcAdapter) UpdateFundingRates() (time.Duration, error) {
	start := time.Now()
	slog.Info("Starting Mexc funding rate update...")

	// 1. Fetch all contract details to get the list of symbols
	resp, err := http.Get(mexcFuturesURL + mexcContractDetailPath)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch Mexc contract details: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read Mexc contract details response: %w", err)
	}

	var detailResponse MexcContractDetailResponse
	if err := json.Unmarshal(body, &detailResponse); err != nil {
		return 0, fmt.Errorf("failed to unmarshal Mexc contract details: %w", err)
	}
	if !detailResponse.Success {
		return 0, fmt.Errorf("Mexc contract details API returned success: false")
	}

	var symbols []string
	for _, detail := range detailResponse.Data {
		symbols = append(symbols, detail.Symbol)
	}
	slog.Info("Fetched all Mexc symbols for funding rates", "count", len(symbols))

	// 2. Fetch funding rates in rate-limited chunks
	const chunkSize = 10
	const delay = 2 * time.Second

	newFundingRates := make(map[string]MexcFundingRateDto)
	var wg sync.WaitGroup
	var mu sync.Mutex // Mutex to protect the newFundingRates map

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute) // Context for HTTP requests
	defer cancel()

	for i := 0; i < len(symbols); i += chunkSize {
		end := i + chunkSize
		if end > len(symbols) {
			end = len(symbols)
		}
		chunk := symbols[i:end]

		slog.Debug("Fetching funding rate chunk", "chunk_size", len(chunk), "start_index", i)

		for _, symbol := range chunk {
			wg.Add(1)
			go func(s string) {
				defer wg.Done()
				url := mexcFuturesURL + mexcFundingRatePath + s
				req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
				if err != nil {
					slog.Warn("Failed to create HTTP request for Mexc funding rate", "symbol", s, "error", err)
					return
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					slog.Warn("Failed to fetch Mexc funding rate", "symbol", s, "error", err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					slog.Warn("Mexc funding rate API returned non-OK status", "symbol", s, "status", resp.StatusCode)
					return
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					slog.Warn("Failed to read Mexc funding rate response body", "symbol", s, "error", err)
					return
				}

				var fundingResponse MexcFundingRateResponse
				if err := json.Unmarshal(body, &fundingResponse); err != nil {
					slog.Warn("Failed to unmarshal Mexc funding rate", "symbol", s, "error", err)
					return
				}

				if fundingResponse.Success {
					unifiedSymbol, err := UnwrapMexcSymbol(fundingResponse.Data.Symbol)
					if err == nil {
						mu.Lock()
						newFundingRates[unifiedSymbol] = fundingResponse.Data
						mu.Unlock()
					}
				} else {
					slog.Warn("Mexc funding rate API returned success: false", "symbol", s, "code", fundingResponse.Code)
				}
			}(symbol)
		}

		// Wait for the current chunk to finish before sleeping
		wg.Wait()

		// If this is not the last chunk, sleep to respect rate limits
		if end < len(symbols) {
			time.Sleep(delay)
		}
	}

	// 3. Atomically update the adapter's funding rates map
	a.mu.Lock()
	a.FundingRates = newFundingRates
	a.mu.Unlock()

	// 4. Persist new funding rates to Redis
	redisCtx, redisCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer redisCancel()
	for unifiedSymbol, dto := range newFundingRates {
		key := redisMexcFundingPrefix + unifiedSymbol
		val, err := json.Marshal(dto)
		if err != nil {
			slog.Error("Failed to marshal Mexc funding rate for Redis", "symbol", unifiedSymbol, "error", err)
			continue
		}
		if err := a.redisClient.Set(redisCtx, key, val, redisTTL).Err(); err != nil {
			slog.Error("Failed to save Mexc funding rate to Redis", "symbol", unifiedSymbol, "error", err)
		}
	}
	slog.Info("Persisted Mexc funding rates to Redis.", "count", len(newFundingRates))

	duration := time.Since(start)
	slog.Info("Mexc funding rate update complete", "duration", duration, "updated_count", len(newFundingRates))
	return duration, nil
}

// GetTickers fetches the latest book tickers from Mexc.
func (a *MexcAdapter) GetTickers() ([]MexcTickerDto, time.Duration, error) {
	start := time.Now()

	resp, err := http.Get(mexcFuturesURL + mexcTickersPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to make HTTP request to Mexc: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("Mexc API returned non-OK status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read Mexc response body: %w", err)
	}

	var mexcResponse MexcTickersResponse
	if err := json.Unmarshal(body, &mexcResponse); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal Mexc tickers: %w", err)
	}

	if !mexcResponse.Success {
		return nil, 0, fmt.Errorf("Mexc API returned success: false, code: %d", mexcResponse.Code)
	}

	duration := time.Since(start)
	return mexcResponse.Data, duration, nil
}

// ToTickerBidAsk converts a MexcTickerDto to a shared.TickerBidAsk.
func (m MexcTickerDto) ToTickerBidAsk() (shared.TickerBidAsk, error) {
	unifiedSymbol, err := UnwrapMexcSymbol(m.Symbol)
	if err != nil {
		return shared.TickerBidAsk{}, fmt.Errorf("failed to unwrap Mexc symbol %s: %w", m.Symbol, err)
	}

	return shared.TickerBidAsk{
		Symbol:        m.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Bid:           m.Bid1,
		Ask:           m.Ask1,
		VolumeUSD:     m.Amount24,
	}, nil
}

// UnwrapMexcSymbol converts a Mexc symbol (e.g., "BTC_USDT") to our unified format (e.g., "BTC/USDT:PERP").
func UnwrapMexcSymbol(mexcSymbol string) (string, error) {
	if !strings.HasSuffix(mexcSymbol, "_USDT") {
		return "", shared.ErrUnsupportedQuoteCurrency
	}
	base := strings.TrimSuffix(mexcSymbol, "_USDT")
	return base + "/USDT:PERP", nil
}
