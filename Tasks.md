# Objective

building arbitrage monitor for Mexc and Binance exchanges

# Adapters structure

every exchange will have 2 files: adapter and types. the type file will contain responses from api, that will eventually be converted to generic "Ticker" structure that will be defined in shared/types.go. The adapter file will contain URL information and will have the functionality to get tickers and then subscribe to updates via websockets. Since the exchanges have different symbol format, we need to define wrap and unwrap functions that basically convert to and from cex types to unified type of our own. example of our type : "BTC/USDT:PERP". Perp means futures, basically for now every symbol will have perp suffix cause we only stick to futures for now.

# Types and urls

Some types will be defined in json and some as rust structs:

- binance futures url: <https://fapi.binance.com>
- tickers endpoint: /fapi/v1/ticker/bookTicker
- the ticker dto from binance get tickers request (just an array of BinanceBookTickerDto)

[
{
"symbol": "BTCUSDT",
"bidPrice": "102190.00",
"bidQty": "0.980",
"askPrice": "102190.10",
"askQty": "5.288",
"time": 1762640310280,
"lastUpdateId": 9132437769237
}
]

- mexc futures url: <https://contract.mexc.com>
- tickers endpoint: /api/v1/contract/ticker
- tickers dto response:
  {
  "success": true,
  "code": 0,
  "data": [
  {
  "contractId": 10,
  "symbol": "BTC_USDT",
  "lastPrice": 102216.5,
  "bid1": 102216.4,
  "ask1": 102216.5,
  "volume24": 1635374558,
  "amount24": 16760979976.52886,
  "holdVol": 509407455,
  "lower24Price": 101414.8,
  "high24Price": 104063.8,
  "riseFallRate": 0.0052,
  "riseFallValue": 531.4,
  "indexPrice": 102256.2,
  "fairPrice": 102216.5,
  "fundingRate": 0.000029,
  "maxBidPrice": 112481.8,
  "minAskPrice": 92030.5,
  "timestamp": 1762641653715,
  "riseFallRates": {
  "zone": "UTC+8",
  "r": 0.0052,
  "v": 531.4,
  "r7": -0.0727,
  "r30": -0.1557,
  "r90": -0.1393,
  "r180": -0.0027,
  "r365": 0.3375
  },
  "riseFallRatesOfTimezone": [
  -0.0156,
  -0.0105,
  0.0052
  ]
  }
  ]
  }

- shared type that we will be converting both to:

# [derive(Debug, Clone)] #[allow(dead_code)]

pub struct TickerBidAsk {
pub symbol: String,
pub unified_symbol: String,
pub bid: f32,
pub ask: f32,
pub volume_usd: f32,
}

for binance just return 1kk volume.

# Step 1

Create adapters for binance and mexc with only constants defined. Mock http functions with some logs. In main.go define a cron job that calls get request in parallel for both exchanges.

# Step 2

Create ticker types based on the types defined in the description.

# Step 3

Lets implement HTTP requests for both exchanges. also it would be nice to have some debugging tool to log execution time of both requests. Log the time alongside the tickers count.

# Step 4

Convert responses to arrays of generic tickers that we've defined, log first 3 of the tickers of each exchanges

# Step 5

create a service that is responsible for calculating arbitrage opportunities. the logic should be derived from the following rust code:

df![
            "exchange" => exchanges,
            "symbol" => symbols,
            "unified_symbol" => unified_symbols,
            "bid" => bids,
            "ask" => asks,
            "volume" => volumes
        ]

pub fn calculate*price_spreads(&mut self) -> PolarsResult<DataFrame> {
let df = self.dataframe.clone();
let spreads = df
.clone()
.join(
df,
[col("unified_symbol")],
[col("unified_symbol")],
JoinArgs::new(JoinType::Inner).with_suffix(Some(PlSmallStr::from_str("\_long"))),
)
// Filter out self-comparisons (exchange == exchange_long)
.filter(col("exchange").neq(col("exchange_long")))
/\_get avg prices */
.with*columns([
((col("bid") + col("ask_long")) / lit(2)).alias("open_avg_price"),
((col("bid_long") + col("ask")) / lit(2)).alias("exit_avg_price"),
])
/* get open/close diffs */
.with_columns([
(col("bid") - col("ask_long")).alias("open_diff"),
(col("bid_long") - col("ask")).alias("exit_diff"),
])
/* calculate spreads\_/
.with_columns([
(col("open_diff") / col("open_avg_price") *lit(100)).alias("entry_spread"),
(col("exit_diff") / col("exit_avg_price")* lit(100)).alias("exit_spread"),
])
.sort_by_exprs(
[col("entry_spread")],
SortMultipleOptions::default().with_order_descending(true),
);
if let Some(filters) = &self.filters {
spreads
.filter(col("entry_spread").gt_eq(filters.spread_threshold))
.collect()
} else {
spreads.collect()
}
}

- keep in mind that currently we only have 2 exchanges, but there may be more later, so please make sure this code works no matter how many exchanges we would have.
