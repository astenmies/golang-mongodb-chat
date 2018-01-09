package models

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/manhtai/golang-mongodb-chat/config"
)

// Room represents a room to chat
type Room struct {
	// forward is a channel that holds incoming messages
	// that should be forwarded to the other clients.
	forward chan *Message
	// join is a channel for clients wishing to join the room.
	join chan *Client
	// leave is a channel for clients wishing to leave the room.
	leave chan *Client
	// clients holds all current clients in this room.
	clients map[*Client]bool
}

// run start a room and run it forever
func run(r *Room) {
	for {
		select {
		case client := <-r.join:
			// joining
			r.clients[client] = true
		case client := <-r.leave:
			// leaving
			delete(r.clients, client)
			close(client.send)
		case msg := <-r.forward:
			// forward message to all clients
			for client := range r.clients {
				client.send <- msg
			}
		}
	}
}

// NewRoomChan creates a new room for clients to join
func NewRoomChan() *Room {
	r := &Room{
		forward: make(chan *Message),
		join:    make(chan *Client),
		leave:   make(chan *Client),
		clients: make(map[*Client]bool),
	}
	go run(r)
	return r
}

const (
	socketBufferSize  = 1024
	messageBufferSize = 256
)

var upgrader = &websocket.Upgrader{ReadBufferSize: socketBufferSize,
	WriteBufferSize: socketBufferSize}

// RoomChat take a room, return a HandlerFunc,
// responsible for send & receive websocket data for all channels
func RoomChat(r *Room, sm *chan SaveMessage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {

		vars := mux.Vars(req)

		socket, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			log.Print("ServeHTTP:", err)
			return
		}

		// Get user out of session
		session, _ := config.Store.Get(req, "session")
		val := session.Values["user"]
		var user = &User{}
		var ok bool
		if user, ok = val.(*User); !ok {
			log.Print("Invalid session")
			return
		}

		client := &Client{
			socket:  socket,
			send:    make(chan *Message, messageBufferSize),
			room:    r,
			user:    user,
			channel: vars["id"],
			save:    sm,
		}

		r.join <- client
		defer func() { r.leave <- client }()
		go client.write()
		client.read()
	}
}
