package adapters

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cex-price-diff-notifications/shared"
)

const (
	binanceFuturesURL     = "https://fapi.binance.com"
	binanceBookTickerPath = "/fapi/v1/ticker/bookTicker"
	binanceFundingRatePath = "/fapi/v1/fundingRate"
)

// BinanceAdapter holds state and logic for interacting with the Binance API.
type BinanceAdapter struct {
	FundingRates map[string]BinanceFundingRateDto
	mu           sync.RWMutex
}

// NewBinanceAdapter creates a new instance of the BinanceAdapter.
func NewBinanceAdapter() *BinanceAdapter {
	return &BinanceAdapter{
		FundingRates: make(map[string]BinanceFundingRateDto),
	}
}

// GetTickers fetches the latest book tickers from Binance.
func (a *BinanceAdapter) GetTickers() ([]BinanceBookTickerDto, time.Duration, error) {
	start := time.Now()
	
	resp, err := http.Get(binanceFuturesURL + binanceBookTickerPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to make HTTP request to Binance tickers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("Binance tickers API returned non-OK status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read Binance tickers response body: %w", err)
	}

	var tickers []BinanceBookTickerDto
	if err := json.Unmarshal(body, &tickers); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal Binance tickers: %w", err)
	}

	duration := time.Since(start)
	return tickers, duration, nil
}

// UpdateFundingRates fetches and stores the latest funding rates from Binance.
func (a *BinanceAdapter) UpdateFundingRates() (time.Duration, error) {
	start := time.Now()

	resp, err := http.Get(binanceFuturesURL + binanceFundingRatePath)
	if err != nil {
		return 0, fmt.Errorf("failed to make HTTP request to Binance funding rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("Binance funding rates API returned non-OK status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read Binance funding rates response body: %w", err)
	}

	var rates []BinanceFundingRateDto
	if err := json.Unmarshal(body, &rates); err != nil {
		return 0, fmt.Errorf("failed to unmarshal Binance funding rates: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, rate := range rates {
		unifiedSymbol, err := UnwrapBinanceSymbol(rate.Symbol)
		if err != nil {
			// Ignore symbols we can't unwrap (e.g., non-USDT pairs)
			continue
		}
		a.FundingRates[unifiedSymbol] = rate
	}

	return time.Since(start), nil
}

// ToTickerBidAsk converts a BinanceBookTickerDto to a shared.TickerBidAsk.
func (b BinanceBookTickerDto) ToTickerBidAsk() (shared.TickerBidAsk, error) {
	unifiedSymbol, err := UnwrapBinanceSymbol(b.Symbol)
	if err != nil {
		return shared.TickerBidAsk{}, fmt.Errorf("failed to unwrap Binance symbol %s: %w", b.Symbol, err)
	}

	bid, err := strconv.ParseFloat(b.BidPrice, 64)
	if err != nil {
		return shared.TickerBidAsk{}, fmt.Errorf("failed to parse Binance bid price %s: %w", b.BidPrice, err)
	}

	ask, err := strconv.ParseFloat(b.AskPrice, 64)
	if err != nil {
		return shared.TickerBidAsk{}, fmt.Errorf("failed to parse Binance ask price %s: %w", b.AskPrice, err)
	}

	// As per instruction, hardcode volume for Binance
	volumeUSD := 1_000_000.0

	return shared.TickerBidAsk{
		Symbol:        b.Symbol,
		UnifiedSymbol: unifiedSymbol,
		Bid:           bid,
		Ask:           ask,
		VolumeUSD:     volumeUSD,
	},
	nil
}

// UnwrapBinanceSymbol converts a Binance symbol (e.g., "BTCUSDT") to our unified format (e.g., "BTC/USDT:PERP").
func UnwrapBinanceSymbol(binanceSymbol string) (string, error) {
	if !strings.HasSuffix(binanceSymbol, "USDT") {
		return "", shared.ErrUnsupportedQuoteCurrency
	}
	base := strings.TrimSuffix(binanceSymbol, "USDT")
	return base + "/USDT:PERP", nil
}
