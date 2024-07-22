package server

import (
	"context"
	"encoding/json"
	"fmt"
	"grain/events"
	"time"

	"grain/utils"

	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/net/websocket"
)

type Subscription struct {
	ID      string
	Filters []Filter
}

// Filter represents the criteria used to query events
type Filter struct {
	IDs     []string            `json:"ids"`
	Authors []string            `json:"authors"`
	Kinds   []int               `json:"kinds"`
	Tags    map[string][]string `json:"tags"`
	Since   *time.Time          `json:"since"`
	Until   *time.Time          `json:"until"`
	Limit   *int                `json:"limit"`
}

var subscriptions = make(map[string]Subscription)
var client *mongo.Client

func SetClient(mongoClient *mongo.Client) {
	client = mongoClient
}

func Handler(ws *websocket.Conn) {
	defer ws.Close()

	var msg string
	for {
		err := websocket.Message.Receive(ws, &msg)
		if err != nil {
			fmt.Println("Error receiving message:", err)
			return
		}
		fmt.Println("Received message:", msg)

		var message []interface{}
		err = json.Unmarshal([]byte(msg), &message)
		if err != nil {
			fmt.Println("Error parsing message:", err)
			continue
		}

		if len(message) < 2 {
			fmt.Println("Invalid message format")
			continue
		}

		messageType, ok := message[0].(string)
		if !ok {
			fmt.Println("Invalid message type")
			continue
		}

		switch messageType {
		case "EVENT":
			handleEvent(ws, message)
		case "REQ":
			handleReq(ws, message)
		case "CLOSE":
			handleClose(ws, message)
		default:
			fmt.Println("Unknown message type:", messageType)
		}
	}
}

func handleEvent(ws *websocket.Conn, message []interface{}) {
	if len(message) != 2 {
		fmt.Println("Invalid EVENT message format")
		return
	}

	eventData, ok := message[1].(map[string]interface{})
	if !ok {
		fmt.Println("Invalid event data format")
		return
	}
	eventBytes, err := json.Marshal(eventData)
	if err != nil {
		fmt.Println("Error marshaling event data:", err)
		return
	}

	var evt events.Event
	err = json.Unmarshal(eventBytes, &evt)
	if err != nil {
		fmt.Println("Error unmarshaling event data:", err)
		return
	}

	events.HandleEvent(context.TODO(), evt, client, ws)

	fmt.Println("Event processed:", evt.ID)
}

func handleReq(ws *websocket.Conn, message []interface{}) {
	if len(message) < 3 {
		fmt.Println("Invalid REQ message format")
		return
	}

	subID, ok := message[1].(string)
	if !ok {
		fmt.Println("Invalid subscription ID format")
		return
	}

	filters := make([]Filter, len(message)-2)
	for i, filter := range message[2:] {
		filterData, ok := filter.(map[string]interface{})
		if !ok {
			fmt.Println("Invalid filter format")
			return
		}

		var f Filter
		f.IDs = utils.ToStringArray(filterData["ids"])
		f.Authors = utils.ToStringArray(filterData["authors"])
		f.Kinds = utils.ToIntArray(filterData["kinds"])
		f.Tags = utils.ToTagsMap(filterData["tags"])
		f.Since = utils.ToTime(filterData["since"])
		f.Until = utils.ToTime(filterData["until"])
		f.Limit = utils.ToInt(filterData["limit"])

		filters[i] = f
	}

	subscriptions[subID] = Subscription{ID: subID, Filters: filters}
	fmt.Println("Subscription added:", subID)

	// Query the database with filters and send back the results
	events, err := QueryEvents(filters, client, "grain", "event-kind1")
	if err != nil {
		fmt.Println("Error querying events:", err)
		return
	}

	for _, evt := range events {
		msg := []interface{}{"EVENT", subID, evt}
		msgBytes, _ := json.Marshal(msg)
		err = websocket.Message.Send(ws, string(msgBytes))
		if err != nil {
			fmt.Println("Error sending event:", err)
			return
		}
	}

	// Indicate end of stored events
	eoseMsg := []interface{}{"EOSE", subID}
	eoseBytes, _ := json.Marshal(eoseMsg)
	err = websocket.Message.Send(ws, string(eoseBytes))
	if err != nil {
		fmt.Println("Error sending EOSE:", err)
		return
	}
}

func handleClose(ws *websocket.Conn, message []interface{}) {
	if len(message) != 2 {
		fmt.Println("Invalid CLOSE message format")
		return
	}

	subID, ok := message[1].(string)
	if !ok {
		fmt.Println("Invalid subscription ID format")
		return
	}

	delete(subscriptions, subID)
	fmt.Println("Subscription closed:", subID)

	closeMsg := []interface{}{"CLOSED", subID, "Subscription closed"}
	closeBytes, _ := json.Marshal(closeMsg)
	err := websocket.Message.Send(ws, string(closeBytes))
	if err != nil {
		fmt.Println("Error sending CLOSE message:", err)
		return
	}
}