package adapters

import (
	"encoding/json"
	"log/slog"

	"github.com/gorilla/websocket"
)

const (
	mexcWebSocketURL       = "wss://contract.mexc.com/edge"
	mexcSubscriptionsPerConn = 20
)

// startWsConnections calculates the required number of WebSocket connections and starts them.
func (a *MexcAdapter) startWsConnections() {
	slog.Info("Starting Mexc WebSocket connections for funding rates...")
	for i := 0; i < len(a.symbols); i += mexcSubscriptionsPerConn {
		end := i + mexcSubscriptionsPerConn
		if end > len(a.symbols) {
			end = len(a.symbols)
		}
		symbolsChunk := a.symbols[i:end]
		go a.manageWsConnection(symbolsChunk)
	}
}

// manageWsConnection handles a single WebSocket connection for a chunk of symbols.
func (a *MexcAdapter) manageWsConnection(symbols []string) {
	conn, _, err := websocket.DefaultDialer.Dial(mexcWebSocketURL, nil)
	if err != nil {
		slog.Error("Failed to connect to Mexc WebSocket", "error", err)
		return
	}
	defer conn.Close()
	slog.Info("Mexc WebSocket connected", "subscribed_symbols", len(symbols))

	// Subscribe to funding rates for the given symbols
	for _, symbol := range symbols {
		subMsg := map[string]interface{}{
			"method": "sub.funding.rate",
			"param":  map[string]string{"symbol": symbol},
		}
		if err := conn.WriteJSON(subMsg); err != nil {
			slog.Error("Failed to subscribe to Mexc funding rate", "symbol", symbol, "error", err)
			return
		}
	}

	// Temporary struct for initial unmarshaling to inspect the 'channel' and 'data' as raw JSON
	type MexcWsTempMessage struct {
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data"`
		Ts      int64           `json:"ts"`
	}

	// Read messages from the connection
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			slog.Error("Error reading from Mexc WebSocket", "error", err)
			return // End goroutine on error
		}

		var tempMsg MexcWsTempMessage
		if err := json.Unmarshal(message, &tempMsg); err != nil {
			slog.Warn("Failed to unmarshal Mexc WebSocket message into temp struct", "error", err, "message", string(message))
			continue
		}

		switch tempMsg.Channel {
		case "push.funding.rate":
			var fundingData MexcFundingRateData
			if err := json.Unmarshal(tempMsg.Data, &fundingData); err != nil {
				slog.Warn("Failed to unmarshal Mexc funding rate data", "error", err, "message", string(tempMsg.Data))
				continue
			}
			unifiedSymbol, err := UnwrapMexcSymbol(fundingData.Symbol)
			if err != nil {
				continue // Ignore symbols we can't process
			}
			a.mu.Lock()
			a.FundingRates[unifiedSymbol] = fundingData
			a.mu.Unlock()
		case "rs.error":
			var errMsg string
			if err := json.Unmarshal(tempMsg.Data, &errMsg); err != nil {
				slog.Error("Received error from Mexc WebSocket, failed to unmarshal error message", "error", err, "message", string(tempMsg.Data))
			} else {
				slog.Error("Received error from Mexc WebSocket", "error_message", errMsg)
			}
		case "rs.sub.funding.rate":
			// This is a subscription acknowledgment, data is typically "success" string. We can ignore it.
		default:
			slog.Warn("Received unknown channel message from Mexc WebSocket", "channel", tempMsg.Channel)
		}
	}
}
