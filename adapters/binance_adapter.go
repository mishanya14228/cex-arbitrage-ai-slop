package adapters

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cex-price-diff-notifications/shared" // Import the shared package
)

const (
	BinanceFuturesURL     = "https://fapi.binance.com"
	BinanceBookTickerPath = "/fapi/v1/ticker/bookTicker"
)

// GetBinanceTickers fetches tickers from Binance.
func GetBinanceTickers() ([]BinanceBookTickerDto, time.Duration, error) {
	start := time.Now()
	
	resp, err := http.Get(BinanceFuturesURL + BinanceBookTickerPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to make HTTP request to Binance: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("Binance API returned non-OK status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read Binance response body: %w", err)
	}

	var tickers []BinanceBookTickerDto
	if err := json.Unmarshal(body, &tickers); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal Binance tickers: %w", err)
	}

	duration := time.Since(start)
	return tickers, duration, nil
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
	}, nil
}

// WrapBinanceSymbol converts a unified symbol (e.g., "BTC/USDT:PERP") to Binance's format (e.g., "BTCUSDT").
func WrapBinanceSymbol(unifiedSymbol string) (string, error) {
	// Expecting format "BASE/USDT:PERP"
	parts := strings.Split(unifiedSymbol, "/")
	if len(parts) != 2 {
		return "", shared.ErrInvalidUnifiedSymbol
	}
	base := parts[0]
	
	quoteAndSuffix := strings.Split(parts[1], ":")
	if len(quoteAndSuffix) != 2 || quoteAndSuffix[0] != "USDT" || quoteAndSuffix[1] != "PERP" {
		return "", shared.ErrInvalidUnifiedSymbol
	}
	quote := quoteAndSuffix[0]

	return base + quote, nil
}

// UnwrapBinanceSymbol converts a Binance symbol (e.g., "BTCUSDT") to our unified format (e.g., "BTC/USDT:PERP").
func UnwrapBinanceSymbol(binanceSymbol string) (string, error) {
	if !strings.HasSuffix(binanceSymbol, "USDT") {
		return "", shared.ErrUnsupportedQuoteCurrency
	}
	base := strings.TrimSuffix(binanceSymbol, "USDT")
	return base + "/USDT:PERP", nil
}
