package http

import (
	"context"
	"github.com/armon/go-metrics"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/logger"
	recover2 "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/woodliu/raft-example/src/raft"
	"github.com/woodliu/raft-example/src/utils"
	"go.uber.org/zap"
	"time"
)

type Server struct {
	app    *fiber.App
	r      raft.Raft
	reg    *prometheus.Registry
	logger *zap.SugaredLogger
}

func StartServer(ctx context.Context, app *fiber.App, r raft.Raft) error {
	// register prometheus registry
	reg, err := utils.NewPromRegistry()
	if err != nil {
		return err
	}

	srv := &Server{
		app:    app,
		r:      r,
		reg:    reg,
		logger: utils.LoggerFromContext(ctx).Sugar(),
	}
	srv.setRouter()
	return nil
}

func (srv *Server) setRouter() {
	srv.app.Use(recover2.New(recover2.Config{
		EnableStackTrace: true,
	}))
	srv.app.Use(logger.New(logger.Config{
		// For more options, see the Config section
		Format: "[${time}] ${status} - ${latency} ${method} ${path} ${body}\n",
	}))

	srv.app.Get("/metrics", adaptor.HTTPHandler(promhttp.HandlerFor(srv.reg, promhttp.HandlerOpts{Registry: srv.reg})))

	appApi := srv.app.Group("/api/v1")
	appApi.Post("/set", srv.set)
	appApi.Get("/get", srv.get)

	operaApi := srv.app.Group("/opera")
	operaApi.Get("/stats", srv.getStats)
}

func (srv *Server) set(c *fiber.Ctx) error {
	s := time.Now()
	err := srv.r.SetValue(c.Body())
	utils.PromSink.AddSampleWithLabels(utils.RaftLatencySummary, float32(time.Since(s).Seconds()), []metrics.Label{{Name: "action", Value: "set"}})
	if err != nil {
		srv.logger.Errorln(err)
		c.WriteString(err.Error())
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	return c.SendStatus(fiber.StatusOK)
}

func (srv *Server) get(c *fiber.Ctx) error {
	req := raft.LogEntryData{}
	if err := c.BodyParser(&req); err != nil {
		srv.logger.Errorln("http request error", err)
		c.Status(fiber.StatusBadRequest)
		return err
	}
	s := time.Now()
	value := srv.r.GetValue(req.Key)
	utils.PromSink.AddSampleWithLabels(utils.RaftLatencySummary, float32(time.Since(s).Seconds()), []metrics.Label{{Name: "action", Value: "get"}})
	c.WriteString(value)
	return c.SendStatus(fiber.StatusOK)
}

func (srv *Server) getStats(c *fiber.Ctx) error {
	stats := srv.r.GetRaftStats()
	return c.Status(fiber.StatusOK).JSON(stats)
}
