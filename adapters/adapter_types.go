package adapters

// BinanceBookTickerDto represents a single ticker response from Binance.
// We only define the fields we need. The json unmarshaller will ignore the rest.
type BinanceBookTickerDto struct {
	Symbol   string `json:"symbol"`
	BidPrice string `json:"bidPrice"`
	AskPrice string `json:"askPrice"`
}

// BinanceFundingRateDto represents a single funding rate response from Binance.
type BinanceFundingRateDto struct {
	Symbol      string `json:"symbol"`
	FundingTime int64  `json:"fundingTime"`
	FundingRate string `json:"fundingRate"`
	MarkPrice   string `json:"markPrice"`
}

// MexcTickerDto represents a single ticker response from Mexc.
// We only define the fields we need.
type MexcTickerDto struct {
	Symbol   string  `json:"symbol"`
	Bid1     float64 `json:"bid1"`
	Ask1     float64 `json:"ask1"`
	Amount24 float64 `json:"amount24"` // This is 'volume24' in the docs, but 'amount24' is volume in USD
}

// MexcTickersResponse represents the full response structure from Mexc's ticker endpoint.
type MexcTickersResponse struct {
	Success bool            `json:"success"`
	Code    int             `json:"code"`
	Data    []MexcTickerDto `json:"data"`
}
