package main

import (
	"bytes"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var (
	newline = []byte("\n")
	space   = []byte(" ")
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Purpose int

const (
	LookUp Purpose = iota
	UpdateValue
	UpdateConfig
)

type Message struct {
	client  *Client
	purpose Purpose
	message []byte
}

type Client struct {
	server *WebsocketServer

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	fileID string
}

func (client *Client) read() {
	defer func() {
		client.server.unregister <- client
		client.conn.Close()
	}()

	client.conn.SetReadLimit(maxMessageSize)
	client.conn.SetReadDeadline(time.Now().Add(pongWait))
	client.conn.SetPongHandler(func(appData string) error {
		client.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Println(err)
			}
			break
		}

		message = bytes.TrimSpace(bytes.ReplaceAll(message, newline, space))
		messageSegments := bytes.Split(message, []byte(" "))
		message = bytes.Join(messageSegments[1:], space)

		var purpose Purpose
		switch string(messageSegments[0]) {
		case "look-up":
			purpose = LookUp
		case "update-value":
			purpose = UpdateValue
		case "update-config":
			purpose = UpdateConfig
		}

		client.server.messages <- Message{client, purpose, message}
	}
}

func (client *Client) write() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		client.conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.send:
			client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			writer, err := client.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Panicf("[ERROR] %s", err.Error())
				return
			}
			writer.Write(message)

			n := len(client.send)
			for i := 0; i < n; i++ {
				writer.Write(newline)
				writer.Write(<-client.send)
			}

			err = writer.Close()
			if err != nil {
				log.Panicf("[ERROR] %s", err.Error())
				return
			}
		case <-ticker.C:
			client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := client.conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				log.Panicf("[ERROR] %s", err.Error())
				return
			}
		}
	}
}

func serveWs(server *WebsocketServer, w http.ResponseWriter, r *http.Request, fileID string) {
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	client := &Client{server, connection, make(chan []byte, 256), fileID}
	client.server.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.write()
	go client.read()
}
