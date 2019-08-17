package websocket

// Message between web app and client
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}
