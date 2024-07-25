package kinds

import (
	"context"
	"grain/relay/handlers/response"
	relay "grain/relay/types"

	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/net/websocket"
)

func HandleUnknownKind(ctx context.Context, evt relay.Event, collection *mongo.Collection, ws *websocket.Conn) error {
	// Respond with an OK message indicating the event is not accepted
	response.SendOK(ws, evt.ID, false, "kind is unknown and not accepted")

	// Return nil as there's no error in the process, just that the event is not accepted
	return nil
}
