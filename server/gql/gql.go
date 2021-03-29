package gql

import (
	"context"
	"sync"
	"time"

	"github.com/gobuffalo/packr/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/graph-gophers/graphql-go"
	jsoniter "github.com/json-iterator/go"
	"github.com/troydota/api.poll.komodohype.dev/server/gql/resolvers"
	"github.com/troydota/api.poll.komodohype.dev/utils"

	log "github.com/sirupsen/logrus"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type GQLRequest struct {
	Query          string                 `json:"query"`
	Variables      map[string]interface{} `json:"variables"`
	OperationName  string                 `json:"operation_name"`
	RequestID      string                 `json:"request_id"`
	SubsctiptionID string                 `json:"subscription_id"`
}

type WSResponse struct {
	Payload        interface{} `json:"payload,omitempty"`
	Error          string      `json:"error,omitempty"`
	RequestID      string      `json:"request_id,omitempty"`
	SubscriptionID string      `json:"sub_id,omitempty"`
}

func GQL(app fiber.Router) {
	gql := app.Group("/gql")

	box := packr.New("gql", "./schema")

	s, err := box.FindString("schema.gql")

	if err != nil {
		panic(err)
	}

	schema := graphql.MustParseSchema(s, resolvers.New(), graphql.UseFieldResolvers())

	gql.Use(func(c *fiber.Ctx) error {
		if c.Method() != "GET" {
			return c.Next()
		}
		// IsWebSocketUpgrade returns true if the client
		// requested upgrade to the WebSocket protocol.
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("ip", c.IP())
			return c.Next()
		}
		return c.SendStatus(426)
	})

	gql.Post("/", func(c *fiber.Ctx) error {
		req := &GQLRequest{}
		err := c.BodyParser(req)
		if err != nil {
			log.Errorf("gql req, err=%v", err)
			return c.Status(400).JSON(fiber.Map{
				"status":  400,
				"message": "Invalid GraphQL Request.",
			})
		}

		if err != nil {
			log.Errorf("session, err=%v", err)
			return c.Status(500).JSON(fiber.Map{
				"status":  500,
				"message": "Failed to get session from store.",
			})
		}

		result := schema.Exec(context.WithValue(context.Background(), utils.Key("ip"), c.IP()), req.Query, req.OperationName, req.Variables)

		status := 200

		if len(result.Errors) > 0 {
			status = 400
		}

		return c.Status(status).JSON(result)
	})

	gql.Get("/", websocket.New(func(c *websocket.Conn) {
		closeChan := make(chan struct{})
		mtx := &sync.Mutex{}
		events := map[string]chan struct{}{}
		go func() {
			for {
				select {
				case <-time.After(60 * time.Second):
					mtx.Lock()
					if err = c.WriteMessage(websocket.TextMessage, utils.S2B("HEARTBEAT")); err != nil {
						mtx.Unlock()
						break
					}
					mtx.Unlock()
				case <-closeChan:
					return
				}
			}
		}()
		defer close(closeChan)
		var (
			mt   int
			msg  []byte
			data []byte
			err  error
		)
		for {
			if mt, msg, err = c.ReadMessage(); err != nil {
				break
			}

			if mt != websocket.TextMessage {
				continue
			}

			req := &GQLRequest{}
			if err = json.Unmarshal(msg, req); err != nil {
				data, err = json.Marshal(WSResponse{
					Error: "invalid request",
				})
				if err != nil {
					log.Errorf("json, err=%v", err)
					continue
				}
				mtx.Lock()
				if err = c.WriteMessage(websocket.TextMessage, data); err != nil {
					mtx.Unlock()
					break
				}
				mtx.Unlock()
				continue
			}

			if req.SubsctiptionID != "" && req.OperationName == "unsubscribe" {
				mtx.Lock()
				if v, ok := events[req.SubsctiptionID]; ok {
					close(v)
				}
				mtx.Unlock()
				continue
			}

			go func() {
				queryCtx, cancel := context.WithCancel(context.WithValue(context.Background(), utils.Key("ip"), c.Locals("ip")))
				result, err := schema.Subscribe(queryCtx, req.Query, req.OperationName, req.Variables)
				if err != nil {
					log.Errorf("gql, err=%v", err)
					cancel()
					data, err = json.Marshal(WSResponse{
						Error: "invalid request",
					})
					if err != nil {
						log.Errorf("json, err=%v", err)
						return
					}
					mtx.Lock()
					c.WriteMessage(websocket.TextMessage, data)
					mtx.Unlock()
					return
				}

				unsub := make(chan struct{})
				id, err := utils.GenerateRandomString(20)
				if err != nil {
					log.Errorf("random, err=%v", err)
					cancel()
					data, err = json.Marshal(WSResponse{
						Error: "internal server err",
					})
					if err != nil {
						log.Errorf("json, err=%v", err)
						return
					}
					mtx.Lock()
					c.WriteMessage(websocket.TextMessage, data)
					mtx.Unlock()
					return
				}
				mtx.Lock()
				events[id] = unsub
				mtx.Unlock()
				ended := make(chan struct{})
				defer close(ended)
				defer func() {
					mtx.Lock()
					delete(events, id)
					mtx.Unlock()
				}()
				go func() {
					select {
					case <-closeChan:
						close(unsub)
					case <-ended:
						close(unsub)
					case <-unsub:
					}
					cancel()
				}()

				for val := range result {
					data, err = json.Marshal(WSResponse{
						Payload:        val,
						RequestID:      req.RequestID,
						SubscriptionID: id,
					})
					if err != nil {
						log.Errorf("json, err=%v", err)
						continue
					}
					mtx.Lock()
					if err = c.WriteMessage(websocket.TextMessage, data); err != nil {
						mtx.Unlock()
						break
					}
					mtx.Unlock()
				}
			}()
		}
	}))
}
