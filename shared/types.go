package shared

import "errors"

// TickerBidAsk represents a unified ticker information with bid and ask prices.
type TickerBidAsk struct {
	Symbol        string // Original exchange symbol (e.g., "BTCUSDT")
	UnifiedSymbol string // Our unified symbol format (e.g., "BTC/USDT:PERP")
	Bid           float64
	Ask           float64
	VolumeUSD     float64
}

// FundingRateInfo holds standardized funding rate information.
type FundingRateInfo struct {
	Rate           float64 `json:"rate"`
	Interval       int     `json:"interval"` // Interval in hours
	NextSettleTime int64   `json:"next_settle_time"`
}

var (
	ErrInvalidUnifiedSymbol     = errors.New("invalid unified symbol format")
	ErrUnsupportedQuoteCurrency = errors.New("unsupported quote currency")
)
