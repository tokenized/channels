package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/tokenized/channels"
	channelsClient "github.com/tokenized/channels/client"
	"github.com/tokenized/channels/cmd/sample_client/sample_client"
	"github.com/tokenized/channels/wallet"
	"github.com/tokenized/config"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/threads"
	spyNodeClient "github.com/tokenized/spynode/pkg/client"
)

type Config struct {
	BaseKey bitcoin.Key `envconfig:"BASE_KEY" json:"base_key" masked:"true"`

	StorageBucket     string `envconfig:"STORAGE_BUCKET" json:"STORAGE_BUCKET"`
	StorageRoot       string `envconfig:"STORAGE_ROOT" json:"STORAGE_ROOT"`
	StorageMaxRetries int    `default:"10" envconfig:"STORAGE_MAX_RETRIES" json:"STORAGE_MAX_RETRIES"`
	StorageRetryDelay int    `default:"2000" envconfig:"STORAGE_RETRY_DELAY" json:"STORAGE_RETRY_DELAY"`

	SpyNode spyNodeClient.Config
	Logger  logger.SetupConfig
}

func main() {
	args := os.Args
	if len(args) < 2 {
		fmt.Printf("Command required: channels_sample <listen, list, receive, ...>\n")
		os.Exit(1)
	}

	// Logging
	ctx := context.Background()

	cfg := Config{}
	if err := config.LoadConfig(ctx, &cfg); err != nil {
		logger.Fatal(ctx, "LoadConfig : %s", err)
	}

	ctx = logger.ContextWithLogSetup(ctx, cfg.Logger)

	logger.Info(ctx, "Starting")
	defer logger.Info(ctx, "Completed")

	// Config
	maskedConfig, err := config.MarshalJSONMaskedRaw(cfg)
	if err != nil {
		logger.Fatal(ctx, "Failed to marshal config : %s", err)
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.JSON("config", maskedConfig),
	}, "Config")

	store, err := storage.CreateStreamStorage(cfg.StorageBucket, cfg.StorageRoot,
		cfg.StorageMaxRetries, cfg.StorageRetryDelay)
	if err != nil {
		logger.Fatal(ctx, "Failed to create storage : %s", err)
	}

	if cfg.SpyNode.ConnectionType != spyNodeClient.ConnectionTypeFull {
		logger.Fatal(ctx, "Spynode connection type must be full to receive data : %s", err)
	}

	spyNodeClient, err := spyNodeClient.NewRemoteClient(&cfg.SpyNode)
	if err != nil {
		logger.Fatal(ctx, "Failed to create spynode remote client : %s", err)
	}

	peerChannelsFactory := peer_channels.NewFactory()

	client := sample_client.NewClient(cfg.BaseKey, store, peerChannelsFactory, spyNodeClient)
	spyNodeClient.RegisterHandler(client)

	if err := client.Load(ctx); err != nil {
		logger.Fatal(ctx, "Failed to load client : %s", err)
	}

	switch args[1] {
	case "listen":
		listen(ctx, client, spyNodeClient, args[2:]...)
	case "list":
		list(ctx, client, args[2:]...)
	case "receive":
		receive(ctx, client, spyNodeClient, args[2:]...)
	case "mark":
		mark(ctx, client, args[2:]...)
	case "establish":
		establish(ctx, client, args[2:]...)
	case "order":
		order(ctx, client, args[2:]...)
	}

	if err := client.Save(ctx); err != nil {
		logger.Error(ctx, "Failed to save client : %s", err)
	}
}

func order(ctx context.Context, client *sample_client.Client, args ...string) {
	if len(args) != 1 {
		fmt.Printf("Missing arguments : channels_sample order <channel_hash>\n")
		return
	}

	channelHash, err := bitcoin.NewHash32FromStr(args[0])
	if err != nil {
		fmt.Printf("Invalid channel hash : %s\n", err)
		return
	}

	channel, err := client.ChannelsClient.GetChannelByHash(*channelHash)
	if err != nil {
		fmt.Printf("Failed to get channel : %s\n", err)
		return
	}

	oneK := uint64(1000)
	purchaseOrder := &channels.PurchaseOrder{
		Items: channels.InvoiceItems{
			{
				ID: bitcoin.Hex("standard"),
				Price: channels.Price{
					Quantity: &oneK,
				},
			},
		},
	}

	if _, err := channel.SendMessage(ctx, purchaseOrder, nil); err != nil {
		fmt.Printf("Failed to send message : %s\n", err)
		return
	}
}

func mark(ctx context.Context, client *sample_client.Client, args ...string) {
	if len(args) != 2 {
		fmt.Printf("Missing arguments : channels_sample mark <channel_hash> <message_id>\n")
		return
	}

	channelHash, err := bitcoin.NewHash32FromStr(args[0])
	if err != nil {
		fmt.Printf("Invalid channel hash : %s\n", err)
		return
	}

	messageID, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		fmt.Printf("Invalid message ID : %s\n", err)
		return
	}

	channel, err := client.ChannelsClient.GetChannelByHash(*channelHash)
	if err != nil {
		fmt.Printf("Failed to get channel : %s\n", err)
		return
	}

	if err := channel.MarkMessageIsProcessed(ctx, messageID); err != nil {
		fmt.Printf("Failed to mark message as processed : %s\n", err)
		return
	}
}

func establish(ctx context.Context, client *sample_client.Client, args ...string) {
	if len(args) != 1 {
		fmt.Printf("Missing channel URL argument : channels_sample establish <channel_url>\n")
		return
	}
	channelURL := args[0]
	baseURL, channelID, err := peer_channels.ParseChannelURL(channelURL)
	if err != nil {
		fmt.Printf("Invalid channel URL : %s", err)
		return
	}

	client.ChannelsClient.SetPeerChannelsURL(baseURL)
	channel, err := client.ChannelsClient.CreateInitialServiceChannel(ctx, wallet.RandomHash())
	if err != nil {
		fmt.Printf("Failed to create service channel : %s\n", err)
		return
	}

	fmt.Printf("Created channel : %s\n", channel.Hash())

	serviceChannels := channels.PeerChannels{
		{
			BaseURL: baseURL,
			ID:      channelID,
		},
	}

	if err := channel.SetOutgoingPeerChannels(serviceChannels); err != nil {
		fmt.Printf("Failed to set peer channel : %s\n", err)
		return
	}

	userName := "UserName"
	userIdentity := channels.Identity{
		Name: &userName,
	}

	initiation := &channels.RelationshipInitiation{
		Configuration: channels.ChannelConfiguration{
			PublicKey:          channel.Key().PublicKey(),
			SupportedProtocols: channelsClient.SupportedProtocols(),
		},
		Identity: userIdentity,
	}

	if _, err := channel.SendMessage(ctx, initiation, nil); err != nil {
		fmt.Printf("Failed to send initiation message : %s\n", err)
		return
	}
}

func receive(ctx context.Context, client *sample_client.Client,
	spClient *spyNodeClient.RemoteClient, args ...string) {

	if len(args) != 1 {
		fmt.Printf("Missing value argument : channels_sample receive <satoshis>\n")
		return
	}

	value, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Printf("Invalid value argument : %s\n", err)
		return
	}

	if err := spClient.Connect(ctx); err != nil {
		fmt.Printf("Failed to connect to spynode : %s\n", err)
		os.Exit(1)
	}
	defer spClient.Close(ctx)

	tx, _, err := client.Wallet.CreateBitcoinReceive(ctx, wallet.RandomHash(), uint64(value))
	if err != nil {
		fmt.Printf("Failed to create bitcoin receive : %s\n", err)
		return
	}

	fmt.Printf("Tx : %s\n", tx.String())

	for _, txout := range tx.Tx.TxOut {
		if txout.Value == 0 {
			continue
		}

		ra, err := bitcoin.RawAddressFromLockingScript(txout.LockingScript)
		if err == nil {
			if err := spyNodeClient.SubscribeAddress(ctx, ra, spClient); err != nil {
				fmt.Printf("Failed to subscribe to address : %s\n", err)
			} else {
				fmt.Printf("Subscribed to address : %s\n", bitcoin.NewAddressFromRawAddress(ra,
					bitcoin.MainNet))
			}
		}
	}
}

func list(ctx context.Context, client *sample_client.Client, args ...string) {
	msgs, err := client.ChannelsClient.GetUnprocessedMessages(ctx)
	if err != nil {
		fmt.Printf("Failed to get unprocessed messages : %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("%d messages:\n", len(msgs))

	for _, msg := range msgs {
		wrap, err := channels.Unwrap(msg.Message.Payload())
		if err != nil {
			fmt.Printf("Failed to get unwrap message : %s\n", err)
			continue
		}

		js, err := json.MarshalIndent(wrap, "", "  ")
		if err != nil {
			fmt.Printf("Failed to get marshal message : %s\n", err)
			continue
		}

		fmt.Printf("Channel %s\n", msg.Channel.Hash())
		fmt.Printf("%s\n", js)
	}
}

func listen(ctx context.Context, client *sample_client.Client,
	spyNodeClient *spyNodeClient.RemoteClient, args ...string) {

	if len(args) > 0 {
		fmt.Printf("No arguments for listen command\n")
		os.Exit(1)
	}

	var wait sync.WaitGroup
	var stopper threads.StopCombiner
	spynodeErrors := make(chan error, 1)
	spyNodeClient.SetListenerErrorChannel(&spynodeErrors)

	spynodeThread := threads.NewThreadWithoutStop("Spynode", spyNodeClient.Connect)
	spynodeThread.SetWait(&wait)

	clientThread := threads.NewThread("Client", client.Run)
	clientThread.SetWait(&wait)
	clientComplete := clientThread.GetCompleteChannel()
	stopper.Add(clientThread)

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)

	spynodeThread.Start(ctx)
	clientThread.Start(ctx)

	select {
	case <-clientComplete:
		logger.Error(ctx, "Client completed : %s", clientThread.Error())

	// case <-spynodeComplete:
	// 	logger.Error(ctx, "SpyNode completed : %s", spynodeThread.Error())

	case <-osSignals:
		logger.Info(ctx, "Shutdown requested")
	}

	stopper.Stop(ctx)
	spyNodeClient.Close(ctx)
	wait.Wait()

	if err := clientThread.Error(); err != nil {
		fmt.Printf("Client thread failed : %s\n", err)
	}

	if err := spynodeThread.Error(); err != nil {
		fmt.Printf("SpyNode thread failed : %s\n", err)
	}
}
