package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"
)

func main() {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")

	if len(os.Args) < 4 {
		logger.Fatal(ctx, "Not enough arguments. Need [URL] [Account] [Token]")
	}

	url := os.Args[1]
	accountID := os.Args[2]
	token := os.Args[3]

	logger.InfoWithFields(ctx, []logger.Field{
		logger.String("url", url),
		logger.String("account", accountID),
	}, "Starting listening to peer channel account")

	listenInterrupt := make(chan interface{})
	listenComplete := make(chan interface{})
	incoming := make(chan peer_channels.Message, 5)

	factory := peer_channels.NewFactory()
	client, err := factory.NewClient(url)
	if err != nil {
		logger.Fatal(ctx, "Failed to create peer channel client : %s", err)
	}

	go func() {
		if err := client.AccountListen(ctx, accountID, token, incoming,
			listenInterrupt); err != nil {
			logger.Error(ctx, "Failed to listen : %s", err)
		}

		close(incoming)
		close(listenComplete)
	}()

	go func() {
		for msg := range incoming {
			js, _ := json.MarshalIndent(msg, "", "  ")
			fmt.Printf("Received message : %s\n", js)

			// processMessage(ctx, msg)

			if err := client.MarkMessages(ctx, msg.ChannelID, token,
				msg.Sequence, true, true); err != nil {
				fmt.Printf("Failed to mark message as read : %s", err)
			}
			fmt.Printf("Marked sequence %d as read\n", msg.Sequence)
		}
	}()

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)

	select {
	case <-listenComplete:
		fmt.Printf("Complete (without interrupt)\n")

	case <-osSignals:
		close(listenInterrupt)

		select {
		case <-listenComplete:
			fmt.Printf("Complete (after interrupt)\n")
		case <-time.After(3 * time.Second):
			fmt.Printf("Shut down timed out\n")
		}
	}
}
