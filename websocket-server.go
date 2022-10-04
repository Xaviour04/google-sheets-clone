package main

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
)

type WebsocketServer struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	messages chan Message

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client
}

func createWebsocketServer() *WebsocketServer {
	return &WebsocketServer{
		messages:   make(chan Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

type cellPos struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

type LookUpRequestStruct struct {
	From cellPos `json:"from"`
	To   cellPos `json:"to"`
}

type LookUpResponseStruct struct {
	From  cellPos    `json:"from"`
	To    cellPos    `json:"to"`
	Items [][]string `json:"items"`
}

type UpdateValueStruct struct {
	NewValue string `json:"value"`
	Row      int    `json:"row"`
	Col      int    `json:"col"`
}

func HandleLookUp(message Message) {
	if string(message.message) == "" {
		return
	}

	request := &LookUpRequestStruct{}
	err := json.Unmarshal(message.message, request)
	if err != nil {
		message.client.send <- []byte(fmt.Sprintf("{\"error\":%q}", err.Error()))
		return
	}

	width := request.To.Col - request.From.Col + 1
	height := request.To.Row - request.From.Row + 1

	from := cellPos{max(0, request.From.Row), max(0, request.From.Col)}
	to := cellPos{
		Row: request.To.Row + 1,
		Col: request.To.Col + 1,
	}
	items := [][]string{}

	coloums := ""
	for c := from.Col; c < to.Col; c++ {
		coloums += fmt.Sprintf(`"%d", `, c)
	}
	coloums = strings.TrimRight(coloums, ", ")

	query := fmt.Sprintf(`SELECT %s FROM %q ORDER BY row LIMIT %d OFFSET %d`, coloums, message.client.fileID, height, from.Row)

	rows, err := db.Query(query)
	if err != nil {
		message.client.send <- []byte(fmt.Sprintf(`{"error":%q, "query":%q}`, err.Error(), query))
		return
	}
	defer rows.Close()

	for rows.Next() {
		untyped_row := make([](interface{}), width)

		for c := 0; c < width; c++ {
			untyped_row[c] = reflect.New(reflect.TypeOf("")).Interface()
		}

		err := rows.Scan(untyped_row...)
		if err != nil {
			message.client.send <- []byte(fmt.Sprintf(`{"error":%q, "query":%q}`, err.Error(), query))
			return
		}

		row := make([]string, width)
		for i, elem := range untyped_row {
			if elem == nil {
				row[i] = "nil"
			} else {
				row[i] = *(elem.(*string))
			}
		}
		items = append(items, row)
	}

	responseMessage, err := json.Marshal(LookUpResponseStruct{from, to, items})
	if err != nil {
		message.client.send <- []byte(fmt.Sprintf("{\"error\":%q}", err.Error()))
		return
	}

	message.client.send <- responseMessage
}

func (server *WebsocketServer) run() {
	for {
		select {
		case client := <-server.register:
			log.Printf("%q joined", client.conn.RemoteAddr().String())
			server.clients[client] = true

		case client := <-server.unregister:
			log.Printf("%q left", client.conn.RemoteAddr().String())
			_, ok := server.clients[client]

			if ok {
				delete(server.clients, client)
				close(client.send)
			}

		case message := <-server.messages:

			addr := message.client.conn.RemoteAddr().String()
			log.Printf("%s -> %s", addr, string(message.message))

			switch message.purpose {
			case UpdateConfig:
				log.Println("Update Config Request")

			case UpdateValue:
				log.Println("Update Value Request")
				request := &UpdateValueStruct{}
				err := json.Unmarshal(message.message, request)
				if err != nil {
					message.client.send <- []byte(fmt.Sprintf("{\"error\":%q}", err.Error()))
					continue
				}

				command := fmt.Sprintf(`UPDATE %q SET "%d"=%q WHERE row=%d`, message.client.fileID, request.Col, request.NewValue, request.Row)
				_, err = db.Exec(command)
				if err != nil {
					message.client.send <- []byte(fmt.Sprintf("{\"error\":%q}", err.Error()))
					continue
				}

			case LookUp:
				log.Println("Look Up Table Request")
				HandleLookUp(message)
			}
		}
	}
}
