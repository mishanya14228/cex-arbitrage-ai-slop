package adapters

// BinanceBookTickerDto represents a single ticker response from Binance.
// We only define the fields we need. The json unmarshaller will ignore the rest.
type BinanceBookTickerDto struct {
	Symbol   string `json:"symbol"`
	BidPrice string `json:"bidPrice"`
	AskPrice string `json:"askPrice"`
}

// BinancePremiumIndexDto represents a single premium index response from Binance.
type BinancePremiumIndexDto struct {
	Symbol          string `json:"symbol"`
	LastFundingRate string `json:"lastFundingRate"`
	NextFundingTime int64  `json:"nextFundingTime"`
}

// BinanceFundingInfoDto represents a single funding info response from Binance.
type BinanceFundingInfoDto struct {
	Symbol               string `json:"symbol"`
	FundingIntervalHours int    `json:"fundingIntervalHours"`
}

// BinanceFundingRateDto represents the combined funding rate information for Binance.
type BinanceFundingRateDto struct {
	Symbol               string `json:"symbol"`
	LastFundingRate      string `json:"lastFundingRate"`
	NextFundingTime      int64  `json:"nextFundingTime"`
	FundingIntervalHours int    `json:"fundingIntervalHours"`
}

// MexcContractDetailDto represents a single contract detail from Mexc.
type MexcContractDetailDto struct {
	Symbol string `json:"symbol"`
}

// MexcContractDetailResponse represents the full response from Mexc's contract detail endpoint.
type MexcContractDetailResponse struct {
	Success bool                    `json:"success"`
	Code    int                     `json:"code"`
	Data    []MexcContractDetailDto `json:"data"`
}

// MexcFundingRateDto represents the funding rate information from Mexc's HTTP endpoint.
type MexcFundingRateDto struct {
	Symbol         string  `json:"symbol"`
	FundingRate    float64 `json:"fundingRate"`
	NextSettleTime int64   `json:"nextSettleTime"`
	CollectCycle   int     `json:"collectCycle"`
}

// MexcFundingRateResponse represents the full response from Mexc's funding rate endpoint.
type MexcFundingRateResponse struct {
	Success bool               `json:"success"`
	Code    int                `json:"code"`
	Data    MexcFundingRateDto `json:"data"`
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
