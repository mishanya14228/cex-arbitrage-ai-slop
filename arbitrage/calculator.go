package arbitrage

import (
	"cex-price-diff-notifications/adapters"
	"cex-price-diff-notifications/shared"
	"log/slog"
	"sort"
	"strconv"
)

// Spread represents a potential arbitrage opportunity between two exchanges.
type Spread struct {
	UnifiedSymbol    string                  `json:"unified_symbol"`
	ExchangeShort    string                  `json:"exchange_short"`              // The exchange to sell on (higher bid).
	ExchangeLong     string                  `json:"exchange_long"`               // The exchange to buy on (lower ask).
	EntrySpread      float64                 `json:"entry_spread"`                // The calculated profit percentage for entering the trade.
	OpenDiff         float64                 `json:"open_diff"`                   // The raw price difference (Bid_Short - Ask_Long).
	ExitSpread       float64                 `json:"exit_spread"`                 // The calculated profit percentage for exiting the trade.
	ExitDiff         float64                 `json:"exit_diff"`                   // The raw price difference (Bid_Long - Ask_Short).
	FundingSpread8h  *float64                `json:"funding_spread_8h,omitempty"` // The 8-hour funding spread.
	FundingRateShort *shared.FundingRateInfo `json:"funding_rate_short,omitempty"`
	FundingRateLong  *shared.FundingRateInfo `json:"funding_rate_long,omitempty"`
}

// CalculateSpreads identifies arbitrage opportunities from a map of tickers and funding rates.
func CalculateSpreads(
	tickers map[string]map[string]shared.TickerBidAsk,
	binanceFundingRates map[string]adapters.BinanceFundingRateDto,
	mexcFundingRates map[string]adapters.MexcFundingRateDto,
) []Spread {
	var spreads []Spread

	// Iterate over each symbol that has prices from at least two exchanges.
	for symbol, exchangeData := range tickers {
		if len(exchangeData) < 2 {
			continue
		}

		// Create a list of exchange names for the current symbol.
		var exchanges []string
		for name := range exchangeData {
			exchanges = append(exchanges, name)
		}

		// Generate all unique pairs of exchanges (A, B) and (B, A).
		for i := 0; i < len(exchanges); i++ {
			for j := 0; j < len(exchanges); j++ {
				if i == j {
					continue // Skip self-comparison.
				}

				exchangeA := exchanges[i] // Exchange where we potentially sell (short)
				exchangeB := exchanges[j] // Exchange where we potentially buy (long)

				tickerA := exchangeData[exchangeA]
				tickerB := exchangeData[exchangeB]

				// --- Entry Spread Calculation (Buy on B, Sell on A) ---
				openDiff := tickerA.Bid - tickerB.Ask
				entrySpread := 0.0
				if openDiff > 0 { // Only calculate if there's a positive difference
					openAvgPrice := (tickerA.Bid + tickerB.Ask) / 2
					if openAvgPrice > 0 {
						entrySpread = (openDiff / openAvgPrice) * 100
					}
				}

				// --- Exit Spread Calculation (Buy on A, Sell on B) ---
				exitDiff := tickerB.Bid - tickerA.Ask
				exitSpread := 0.0
				exitAvgPrice := (tickerB.Bid + tickerA.Ask) / 2
				if exitAvgPrice > 0 {
					exitSpread = (exitDiff / exitAvgPrice) * 100
				}

				// --- Funding Rate Calculation ---
				var fundingSpread8h *float64
				fundingInfoA, foundA := getFundingRateInfo(symbol, exchangeA, binanceFundingRates, mexcFundingRates)
				fundingInfoB, foundB := getFundingRateInfo(symbol, exchangeB, binanceFundingRates, mexcFundingRates)

				if foundA && foundB && fundingInfoA.Interval > 0 && fundingInfoB.Interval > 0 {
					// PnL = side * r * (8 / N)
					pnlShort := +1.0 * fundingInfoA.Rate * (8.0 / float64(fundingInfoA.Interval))
					pnlLong := -1.0 * fundingInfoB.Rate * (8.0 / float64(fundingInfoB.Interval))
					totalFundingPnL := (pnlShort + pnlLong) * 100
					fundingSpread8h = &totalFundingPnL
				}

				// Only add a spread if there's a potential entry opportunity
				if entrySpread > 0 {
					spreads = append(spreads, Spread{
						UnifiedSymbol:    symbol,
						ExchangeShort:    exchangeA,
						ExchangeLong:     exchangeB,
						EntrySpread:      entrySpread,
						OpenDiff:         openDiff,
						ExitSpread:       exitSpread,
						ExitDiff:         exitDiff,
						FundingSpread8h:  fundingSpread8h,
						FundingRateShort: fundingInfoA,
						FundingRateLong:  fundingInfoB,
					})
				}
			}
		}
	}

	// Sort spreads by the highest entry percentage, descending.
	sort.Slice(spreads, func(i, j int) bool {
		return spreads[i].EntrySpread > spreads[j].EntrySpread
	})

	return spreads
}

// getFundingRateInfo retrieves the standardized funding rate info for a given symbol and exchange.
func getFundingRateInfo(
	unifiedSymbol string,
	exchangeName string,
	binanceFundingRates map[string]adapters.BinanceFundingRateDto,
	mexcFundingRates map[string]adapters.MexcFundingRateDto,
) (*shared.FundingRateInfo, bool) {
	switch exchangeName {
	case "Binance":
		if dto, ok := binanceFundingRates[unifiedSymbol]; ok {
			r, err := strconv.ParseFloat(dto.LastFundingRate, 64)
			if err != nil {
				slog.Warn("Failed to parse Binance funding rate", "symbol", unifiedSymbol, "rate_str", dto.LastFundingRate, "error", err)
				return nil, false
			}
			return &shared.FundingRateInfo{
				Rate:           r,
				Interval:       dto.FundingIntervalHours,
				NextSettleTime: dto.NextFundingTime,
			}, true
		}
	case "Mexc":
		if dto, ok := mexcFundingRates[unifiedSymbol]; ok {
			return &shared.FundingRateInfo{
				Rate:           dto.FundingRate,
				Interval:       dto.CollectCycle,
				NextSettleTime: dto.NextSettleTime,
			}, true
		}
	}
	return nil, false
}
