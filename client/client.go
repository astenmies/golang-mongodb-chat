package client

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/manhtai/cusbot/message"
	"github.com/manhtai/cusbot/user"
)

// Client represents a user connect to a room, one user may have many devices to chat,
// so it should not be the same as user
type Client struct {
	// socket is the web socket for this client.
	socket *websocket.Conn
	// send is a channel on which messages are sent.
	send chan *message.Message
	// room is the room this client is chatting in.
	room *Room
	user *user.User
}

func (c *Client) read() {
	defer c.socket.Close()
	for {
		var msg *message.Message
		err := c.socket.ReadJSON(&msg)
		if err != nil {
			return
		}
		msg.When = time.Now()

		// Default nick name to Anonymous
		if len(c.user.Name) == 0 {
			c.user.Name = "User #" + strconv.Itoa(c.user.ID)
		}

		// Allow to change nick name
		nick := "/nick "
		if strings.HasPrefix(msg.Body, nick) && len(msg.Body[len(nick):]) > 0 {
			c.user.Name = msg.Body[len(nick):]
			msg.Name = c.user.Name
			msg.Body = "Your nick now is " + c.user.Name
			c.send <- msg
		} else {
			msg.Name = c.user.Name
			c.room.forward <- msg
		}
	}
}

func (c *Client) write() {
	defer c.socket.Close()
	for msg := range c.send {
		err := c.socket.WriteJSON(msg)
		if err != nil {
			return
		}
	}
}

// Room represents a room to chat
type Room struct {
	// forward is a channel that holds incoming messages
	// that should be forwarded to the other clients.
	forward chan *message.Message
	// join is a channel for clients wishing to join the room.
	join chan *Client
	// leave is a channel for clients wishing to leave the room.
	leave chan *Client
	// clients holds all current clients in this room.
	clients map[*Client]bool
}

// Run start a room and run it forever
func (r *Room) Run() {
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

// NewRoom creates a new room for clients to join
func NewRoom() *Room {
	return &Room{
		forward: make(chan *message.Message),
		join:    make(chan *Client),
		leave:   make(chan *Client),
		clients: make(map[*Client]bool),
	}
}

const (
	socketBufferSize  = 1024
	messageBufferSize = 256
)

var upgrader = &websocket.Upgrader{ReadBufferSize: socketBufferSize,
	WriteBufferSize: socketBufferSize}

func (r *Room) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	socket, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Fatal("ServeHTTP:", err)
		return
	}

	user := &user.User{
		ID: len(r.clients) + 1,
	}

	client := &Client{
		socket: socket,
		send:   make(chan *message.Message, messageBufferSize),
		room:   r,
		user:   user,
	}

	r.join <- client
	defer func() { r.leave <- client }()
	go client.write()
	client.read()
}
