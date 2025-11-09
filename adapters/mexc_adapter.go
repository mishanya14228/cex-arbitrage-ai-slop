package adapters

import (
	"context" // Added context import
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"cex-price-diff-notifications/shared"
)

const (
	mexcFuturesURL         = "https://contract.mexc.com"
	mexcContractDetailPath = "/api/v1/contract/detail"
	mexcTickersPath        = "/api/v1/contract/ticker"
)

// MexcAdapter holds state and logic for interacting with the Mexc API.
type MexcAdapter struct {
	FundingRates map[string]MexcFundingRateData
	mu           sync.RWMutex
	symbols      []string

	ctx    context.Context
	cancel context.CancelFunc
}

// NewMexcAdapter creates a new instance of the MexcAdapter and starts WebSocket connections.
func NewMexcAdapter() (*MexcAdapter, error) {
	slog.Info("Initializing Mexc adapter...")

	adapter := &MexcAdapter{
		FundingRates: make(map[string]MexcFundingRateData),
	}

	// Initial setup of context
	adapter.ctx, adapter.cancel = context.WithCancel(context.Background())

	if err := adapter.initConnections(); err != nil {
		adapter.cancel() // Ensure context is cancelled on init failure
		return nil, err
	}

	return adapter, nil
}

// initConnections fetches contract details and starts WebSocket connections.
// This is called by NewMexcAdapter and Restart.
func (a *MexcAdapter) initConnections() error {
	// 1. Fetch all contract details to get the list of symbols
	resp, err := http.Get(mexcFuturesURL + mexcContractDetailPath)
	if err != nil {
		return fmt.Errorf("failed to fetch Mexc contract details: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Mexc contract details response: %w", err)
	}

	var detailResponse MexcContractDetailResponse
	if err := json.Unmarshal(body, &detailResponse); err != nil {
		return fmt.Errorf("failed to unmarshal Mexc contract details: %w", err)
	}
	if !detailResponse.Success {
		return fmt.Errorf("Mexc contract details API returned success: false")
	}

	a.symbols = nil // Clear old symbols
	for _, detail := range detailResponse.Data {
		a.symbols = append(a.symbols, detail.Symbol)
	}
	slog.Info("Fetched all Mexc symbols", "count", len(a.symbols))

	// Clear old funding rates
	a.mu.Lock()
	a.FundingRates = make(map[string]MexcFundingRateData)
	a.mu.Unlock()

	// Start WebSocket connections in the background
	a.startWsConnections(a.ctx) // Pass the adapter's context

	return nil
}

// Restart closes existing connections and re-initializes them.
func (a *MexcAdapter) Restart() error {
	slog.Info("Restarting Mexc adapter connections...")

	// 1. Cancel the old context to stop existing goroutines
	a.cancel()
	// Give some time for goroutines to shut down gracefully
	time.Sleep(1 * time.Second)

	// 2. Create a new context for new goroutines
	a.ctx, a.cancel = context.WithCancel(context.Background())

	// 3. Re-initialize connections
	if err := a.initConnections(); err != nil {
		a.cancel() // Ensure context is cancelled on init failure
		return fmt.Errorf("failed to re-initialize Mexc connections during restart: %w", err)
	}
	slog.Info("Mexc adapter connections restarted successfully.")
	return nil
}

// Close cancels the adapter's context, stopping all associated goroutines.
func (a *MexcAdapter) Close() {
	slog.Info("Closing Mexc adapter...")
	a.cancel()
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
