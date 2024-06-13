package main

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/woodliu/raft-example/cli"
	"github.com/woodliu/raft-example/src/http"
	"github.com/woodliu/raft-example/src/raft"
	"github.com/woodliu/raft-example/src/utils"
	_ "go.uber.org/automaxprocs"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := utils.CreateLogger()
	loggerCtx := utils.ContextWithLogger(context.Background(), logger)

	raftSrv := raft.NewRaft(loggerCtx, &raft.Opts{
		RaftAddress: cli.RaftAddress,
		RpcAddress:  cli.RpcAddress,

		DataDir:   cli.DataDir,
		Bootstrap: cli.Bootstrap,

		SerfAddress:       cli.SerfAddress,
		SerfJoinAddresses: cli.SerfJoinAddresses.Value(),
	})

	app := fiber.New(fiber.Config{
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
	})

	err := http.StartServer(loggerCtx, app, raftSrv)
	if err != nil {
		logger.Sugar().Panic(err)
	}

	go func() {
		if err := app.Listen(cli.HttpAddress); err != nil {
			logger.Sugar().Panic(err)
		}
	}()

	c := make(chan os.Signal, 10)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-c

	logger.Sugar().Infoln("system shutdown...")
	app.ShutdownWithTimeout(time.Second * 3)

	raftSrv.Shutdown(loggerCtx)
	time.Sleep(time.Second * 3)
	os.Exit(0)
}
