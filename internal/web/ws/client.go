package ws

import (
	"encoding/json"
	"log"
	"runtime/debug"
	"time"

	"github.com/Lafetz/showdown-trivia-game/internal/core/game"
	render "github.com/Lafetz/showdown-trivia-game/internal/web/Render"
	"github.com/gorilla/websocket"
)

var (
	pongWait     = 10 * time.Second
	pingInterval = (pongWait * 9) / 10
)

type ClientList map[*Client]bool
type Client struct {
	Username   string
	connection *websocket.Conn
	room       *Room
	egress     chan []byte
}

func NewClient(conn *websocket.Conn, room *Room, username string) *Client {
	c := &Client{
		Username:   username,
		connection: conn,
		room:       room,
		egress:     make(chan []byte),
	}

	return c
}
func (c *Client) sendErrormsg(err error) {
	c.room.hub.Logger.Printf("%s\n%s", err.Error(), debug.Stack())
	bufferr := render.WsServerError()
	c.egress <- bufferr.Bytes()

}
func (c *Client) readMessage() {
	defer func() {
		c.room.removeClient(c)
	}()
	if err := c.connection.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		log.Println(err)
		return
	}
	c.connection.SetReadLimit(512)
	c.connection.SetPongHandler(c.pongHandler)
	for {
		_, msg, err := c.connection.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure) {

				c.sendErrormsg(err)
			}
			break
		}

		var req Event
		if err := json.Unmarshal(msg, &req); err != nil {
			c.sendErrormsg(err)
			break
		}
		//check if valid
		//
		switch req.EventType {
		case StartGame:
			var players []*game.Player
			for c := range c.room.clients {
				players = append(players, game.NewPlayer(c.Username))
			}

			go c.room.Game.Start(players)
		case SendAnswer:
			answer := game.NewAnswer(c.Username, req.Payload)
			c.room.Game.AnswerCh <- answer
			buff := render.RenderUserAnswer(req.Payload)
			c.egress <- buff.Bytes()
		default:
			c.sendErrormsg(err)
		}

	}
}
func (c *Client) writeMessage() {
	defer func() {
		c.room.removeClient(c)
	}()
	ticker := time.NewTicker(pingInterval)

	for {
		select {
		case message, ok := <-c.egress:

			if !ok {
				if err := c.connection.WriteMessage(websocket.CloseMessage, nil); err != nil {
					c.sendErrormsg(err)
				}
				return
			}

			if err := c.connection.WriteMessage(websocket.TextMessage, message); err != nil {
				c.sendErrormsg(err)
			}

		case <-ticker.C:

			if err := c.connection.WriteMessage(websocket.PingMessage, []byte("")); err != nil {
				return
			}
		}
	}
}
func (c *Client) pongHandler(pongMsg string) error {

	return c.connection.SetReadDeadline(time.Now().Add(pongWait))
}
