package main

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	recover2 "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/woodliu/raft-example/cmd"
	"github.com/woodliu/raft-example/src/service"
	_ "go.uber.org/automaxprocs"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cmd.Execute()

	srv := service.SetUpServer()
	app := fiber.New()
	app.Use(recover2.New(recover2.Config{
		EnableStackTrace: true,
	}))
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${latency} ${method} ${path} ${body}\n",
	}))

	v1 := app.Group("/api/v1")
	v1.Get("/set", srv.SetValue)
	v1.Post("/get", srv.GetValue)

	maintain := app.Group("/api/maintain")
	maintain.Get("/stats", srv.GetRaftStats)

	// wait for signal
	go handleSignal(srv)

	log.Fatal(app.Listen(cmd.HttpAddress))
}

func handleSignal(srv service.Service) {
	// wait for signal
	signalCh := make(chan os.Signal, 10)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	for s := range signalCh {
		switch s {
		case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			log.Println("system shutdown...")
			srv.Shutdown()
		default:
			fmt.Println("other signal", s)
		}
	}
}
