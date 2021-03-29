package server

import (
	"net"

	"github.com/davecgh/go-spew/spew"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/troydota/api.poll.komodohype.dev/configure"
	"github.com/troydota/api.poll.komodohype.dev/server/gql"
	"github.com/troydota/api.poll.komodohype.dev/utils"

	log "github.com/sirupsen/logrus"
)

type Server struct {
	app *fiber.App
	ln  net.Listener
}

type customLogger struct{}

func (*customLogger) Write(data []byte) (n int, err error) {
	log.Debugln(utils.B2S(data))
	return len(data), nil
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func NewServer() *Server {
	ln, err := net.Listen(configure.Config.GetString("listener_network"), configure.Config.GetString("listener_address"))
	checkErr(err)

	server := &Server{
		ln: ln,
		app: fiber.New(fiber.Config{
			ErrorHandler: errorHandler,
		}),
	}

	server.app.Use(recover.New())
	server.app.Use(cors.New())
	server.app.Use(logger.New(logger.Config{
		Output: &customLogger{},
	}))

	gql.GQL(server.app)

	server.app.Use(func(c *fiber.Ctx) error {
		return c.Status(404).JSON(&fiber.Map{
			"status":  404,
			"message": "We don't know what you're looking for.",
		})
	})

	go func() {
		err = server.app.Listener(server.ln)
		if err != nil {
			log.Errorf("failed to start http server, err=%v", err)
		}
	}()

	return server
}

func errorHandler(c *fiber.Ctx, err error) error {
	log.Errorf("internal err=%v", spew.Sdump(err))

	return c.SendStatus(500)
}

func (s *Server) Shutdown() error {
	return s.ln.Close()
}
