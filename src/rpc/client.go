package rpc

import (
	"context"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"os"
)

var logger = zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()

func CreateLeaderConn(leaderAddress string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(leaderAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func ForwardRequest(conn *grpc.ClientConn, data []byte) error {
	defer conn.Close()

	req := RequestForward{
		Data: data,
	}

	_, err := NewForwardClient(conn).Route(context.Background(), &req)
	if err != nil {
		fromError, ok := status.FromError(err)
		if !ok {
			logger.Fatal().Stack().Msg(err.Error())
		}
		for _, detail := range fromError.Details() {
			detail1 := detail.(*RequestForwardResponse)
			logger.Error().Msg(detail1.Message)
		}

		return err
	}

	return nil
}
