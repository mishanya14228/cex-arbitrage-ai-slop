package adapters

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cex-price-diff-notifications/shared"
)

const (
	mexcFuturesURL     = "https://contract.mexc.com"
	mexcBookTickerPath = "/api/v1/contract/ticker"
)

// MexcAdapter holds state and logic for interacting with the Mexc API.
type MexcAdapter struct {
	// No state needed yet, but the struct is here for consistency.
}

// NewMexcAdapter creates a new instance of the MexcAdapter.
func NewMexcAdapter() *MexcAdapter {
	return &MexcAdapter{}
}

// GetTickers fetches the latest book tickers from Mexc.
func (a *MexcAdapter) GetTickers() ([]MexcTickerDto, time.Duration, error) {
	start := time.Now()

	resp, err := http.Get(mexcFuturesURL + mexcBookTickerPath)
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
