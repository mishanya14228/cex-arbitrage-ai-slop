package adapters

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cex-price-diff-notifications/shared"
)

const (
	binanceFuturesURL       = "https://fapi.binance.com"
	binanceBookTickerPath   = "/fapi/v1/ticker/bookTicker"
	binancePremiumIndexPath = "/fapi/v1/premiumIndex"
	binanceFundingInfoPath  = "/fapi/v1/fundingInfo"
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

// UpdateFundingRates fetches and stores the latest funding rates from Binance in parallel.
func (a *BinanceAdapter) UpdateFundingRates() (time.Duration, error) {
	start := time.Now()
	var wg sync.WaitGroup
	var errPremium, errInfo error
	var premiumIndexes []BinancePremiumIndexDto
	var fundingInfos []BinanceFundingInfoDto

	wg.Add(2)

	// Fetch Premium Index in a goroutine
	go func() {
		defer wg.Done()
		resp, err := http.Get(binanceFuturesURL + binancePremiumIndexPath)
		if err != nil {
			errPremium = fmt.Errorf("failed to make HTTP request to Binance premium index: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			errPremium = fmt.Errorf("Binance premium index API returned non-OK status: %d, body: %s", resp.StatusCode, string(bodyBytes))
			return
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			errPremium = fmt.Errorf("failed to read Binance premium index response body: %w", err)
			return
		}

		if err := json.Unmarshal(body, &premiumIndexes); err != nil {
			errPremium = fmt.Errorf("failed to unmarshal Binance premium indexes: %w", err)
		}
	}()

	// Fetch Funding Info in a goroutine
	go func() {
		defer wg.Done()
		resp, err := http.Get(binanceFuturesURL + binanceFundingInfoPath)
		if err != nil {
			errInfo = fmt.Errorf("failed to make HTTP request to Binance funding info: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			errInfo = fmt.Errorf("Binance funding info API returned non-OK status: %d, body: %s", resp.StatusCode, string(bodyBytes))
			return
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			errInfo = fmt.Errorf("failed to read Binance funding info response body: %w", err)
			return
		}

		if err := json.Unmarshal(body, &fundingInfos); err != nil {
			errInfo = fmt.Errorf("failed to unmarshal Binance funding infos: %w", err)
		}
	}()

	wg.Wait()

	if errPremium != nil {
		return 0, errPremium
	}
	if errInfo != nil {
		return 0, errInfo
	}

	// Create a map for quick lookup of funding intervals
	fundingInfoMap := make(map[string]BinanceFundingInfoDto)
	for _, info := range fundingInfos {
		fundingInfoMap[info.Symbol] = info
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	loggedCount := 0
	for _, premiumIndex := range premiumIndexes {
		unifiedSymbol, err := UnwrapBinanceSymbol(premiumIndex.Symbol)
		if err != nil {
			continue
		}

		combinedRate := BinanceFundingRateDto{
			Symbol:          premiumIndex.Symbol,
			LastFundingRate: premiumIndex.LastFundingRate,
			NextFundingTime: premiumIndex.NextFundingTime,
		}

		if info, ok := fundingInfoMap[premiumIndex.Symbol]; ok {
			combinedRate.FundingIntervalHours = info.FundingIntervalHours
		} else {
			combinedRate.FundingIntervalHours = 8 // Default to 8 hours
		}
		a.FundingRates[unifiedSymbol] = combinedRate

		if loggedCount < 2 {
			slog.Info("Combined Binance funding rate", "data", combinedRate)
			loggedCount++
		}
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
