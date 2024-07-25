package kinds

import (
	"context"
	"grain/relay/handlers/response"
	relay "grain/relay/types"

	"golang.org/x/net/websocket"
)

func HandleKind2Deprecated(ctx context.Context, evt relay.Event, ws *websocket.Conn) error {
	response.SendNotice(ws, evt.PubKey, "kind 2 is deprecated, event not accepted to the relay, please use kind 10002 as defined in NIP-65")
	return nil
}
