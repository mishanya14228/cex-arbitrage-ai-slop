package arbitrage

import (
	"sort"

	"cex-price-diff-notifications/shared"
)

// Spread represents a potential arbitrage opportunity between two exchanges.
type Spread struct {
	UnifiedSymbol   string   `json:"unified_symbol"`
	ExchangeShort   string   `json:"exchange_short"`    // The exchange to sell on (higher bid).
	ExchangeLong    string   `json:"exchange_long"`     // The exchange to buy on (lower ask).
	EntrySpread     float64  `json:"entry_spread"`      // The calculated profit percentage for entering the trade.
	OpenDiff        float64  `json:"open_diff"`         // The raw price difference (Bid_Short - Ask_Long).
	ExitSpread      float64  `json:"exit_spread"`       // The calculated profit percentage for exiting the trade.
	ExitDiff        float64  `json:"exit_diff"`         // The raw price difference (Bid_Long - Ask_Short).
	FundingSpread8h *float64 `json:"funding_spread_8h"` // The 8-hour funding spread.
}

// CalculateSpreads identifies arbitrage opportunities from a map of tickers.
// The input map is structured as: map[symbol] -> map[exchangeName] -> Ticker.
func CalculateSpreads(tickers map[string]map[string]shared.TickerBidAsk) []Spread {
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
				// This is for the reverse trade, or closing the position.
				// bid_B - ask_A
				exitDiff := tickerB.Bid - tickerA.Ask
				exitSpread := 0.0
				exitAvgPrice := (tickerB.Bid + tickerA.Ask) / 2
				if exitAvgPrice > 0 {
					exitSpread = (exitDiff / exitAvgPrice) * 100
				}

				// Only add a spread if there's a potential entry opportunity
				if entrySpread > 0 {
					spreads = append(spreads, Spread{
						UnifiedSymbol: symbol,
						ExchangeShort: exchangeA,
						ExchangeLong:  exchangeB,
						EntrySpread:   entrySpread,
						OpenDiff:      openDiff,
						ExitSpread:    exitSpread,
						ExitDiff:      exitDiff,
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
