package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// Owner/players have different max message sizes
const OWNER_MAX_MESSAGE_SIZE = 131072
const PLAYER_MAX_MESSAGE_SIZE = 512

// ClientList is a map used to help manage a map of clients
type ClientList map[*Client]bool

// Client is a websocket client, basically a frontend visitor
type Client struct {
	// the websocket connection
	connection *websocket.Conn
	name       string
	lobby      *Lobby

	// manager used to manage the client
	manager *Manager
	// egress is used to avoid concurrent writes on the WebSocket
	egress chan Event
}

var (
	// pongWait is how long we will await a pong response from client
	pongWait     = 10 * time.Second
	pingInterval = (pongWait * 9) / 10
)

// NewClient is used to initialize a new Client with all required values initialized
func NewClient(conn *websocket.Conn, manager *Manager, lobby *Lobby, otp string) *Client {
	return &Client{
		connection: conn,
		manager:    manager,
		lobby:      lobby,
		name:       lobby.otpMapping[otp],
		egress:     make(chan Event),
	}
}

// readMessages will start the client to read messages and handle them
// appropriatly.
// This is suppose to be ran as a goroutine
func (c *Client) readMessages() {
	defer func() {
		// Graceful close the connection once this function is done
		c.lobby.removeClient(c)
	}()

	var maxMessageSize int64 = PLAYER_MAX_MESSAGE_SIZE
	if c.lobby.owner == &c.name {
		maxMessageSize = OWNER_MAX_MESSAGE_SIZE
	}

	// Set max size of messages in bytes
	c.connection.SetReadLimit(maxMessageSize)

	// Configure wait time for pong response, use `current time + pongWait`
	// This has to be done here to set the first initial timer
	if err := c.connection.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		log.Println(err)
		return
	}

	// Configure how to handle pong responses
	c.connection.SetPongHandler(c.pongHandler)

	// Loop Forever
	for {
		// ReadMessage is used to read the next message in queue in the connection
		_, payload, err := c.connection.ReadMessage()
		if err != nil {
			// If connection is closed, we will receive an error here
			// We only want to log strange errors, and have simple disconnection
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error reading message: %v", err)
			}
			break // Break the loop to close connection & clean-up
		}
		// Marshal incoming data into a event struct
		var request Event
		if err := json.Unmarshal(payload, &request); err != nil {
			log.Printf("Error marshalling message: %v", err)
			break
		}
		// Route the Event
		if err := c.manager.routeEvent(request, c); err != nil {
			log.Println("Error handling message: ", err)
		}
	}
}

// pongHandler is used to handle PongMessages for the Client
func (c *Client) pongHandler(pongMsg string) error {
	// Current time + Pong Wait time
	return c.connection.SetReadDeadline(time.Now().Add(pongWait))
}

// Listens for new messages to output to the Client
func (c *Client) writeMessages() {
	// Create a ticker that triggers a ping at given interval
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		// Graceful close if this triggers a closing
		c.lobby.removeClient(c)
	}()

	for {
		select {
		case message, ok := <-c.egress:
			// Ok will be false if the egress channel is closed
			if !ok {
				// Manager has closed this connection channel, so communicate that to frontend
				if err := c.connection.WriteMessage(websocket.CloseMessage, nil); err != nil {
					// Log that the connection is closed and the reason
					log.Println("connection closed: ", err)
				}
				// Return to close the goroutine
				return
			}

			data, err := json.Marshal(message)
			if err != nil {
				log.Println(err)
				return // TODO: do we need to close the connection?
			}
			// Write a regular text message to the connection
			if err := c.connection.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Println(err)
			}
		case <-ticker.C:
			// Send the Ping
			if err := c.connection.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				log.Println("writemsg: ", err)
				return // return to break this goroutine triggering cleanup
			}
		}

	}
}
