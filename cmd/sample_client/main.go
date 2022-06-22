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
	"github.com/tokenized/channels/client"
	channelsClient "github.com/tokenized/channels/client"
	"github.com/tokenized/channels/cmd/sample_client/sample_client"
	"github.com/tokenized/channels/invoices"
	"github.com/tokenized/channels/relationships"
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
		fmt.Printf("Command required: channels_sample <listen, list, display, receive, mark, establish, order, transfer, get_tx>\n")
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

	protocols := client.BuildChannelsProtocols()
	peerChannelsFactory := peer_channels.NewFactory()

	client := sample_client.NewClient(cfg.BaseKey, store, protocols, peerChannelsFactory,
		spyNodeClient, spyNodeClient, spyNodeClient)
	spyNodeClient.RegisterHandler(client)

	if err := client.Load(ctx); err != nil {
		logger.Fatal(ctx, "Failed to load client : %s", err)
	}

	switch args[1] {
	case "listen":
		listen(ctx, client, spyNodeClient, args[2:]...)
	case "list":
		list(ctx, protocols, client, args[2:]...)
	case "display":
		display(ctx, protocols, client, args[2:]...)
	case "receive":
		receive(ctx, client, spyNodeClient, args[2:]...)
	case "mark":
		mark(ctx, client, args[2:]...)
	case "establish":
		establish(ctx, client, args[2:]...)
	case "order":
		order(ctx, client, args[2:]...)
	case "transfer":
		transfer(ctx, protocols, client, spyNodeClient, args[2:]...)
	case "get_tx":
		get_tx(ctx, client, spyNodeClient, args[2:]...)
	}

	if err := client.Save(ctx); err != nil {
		logger.Error(ctx, "Failed to save client : %s", err)
	}
}

func get_tx(ctx context.Context, client *sample_client.Client,
	spClient *spyNodeClient.RemoteClient, args ...string) {

	if len(args) != 1 {
		fmt.Printf("Missing arguments : channels_sample get_tx <txid>\n")
		return
	}

	txid, err := bitcoin.NewHash32FromStr(args[0])
	if err != nil {
		fmt.Printf("Invalid txid : %s\n", err)
		return
	}

	if err := spClient.Connect(ctx); err != nil {
		fmt.Printf("Failed to connect to spynode : %s\n", err)
		return
	}
	defer spClient.Close(ctx)

	tx, err := spClient.GetTx(ctx, *txid)
	if err != nil {
		fmt.Printf("Failed to get tx : %s\n", err)
		return
	}

	if err := client.Wallet.AddTxWithoutContext(ctx, tx); err != nil {
		fmt.Printf("Failed to add tx : %s\n", err)
		return
	}
}

func transfer(ctx context.Context, protocols *channels.Protocols, client *sample_client.Client,
	spClient *spyNodeClient.RemoteClient, args ...string) {
	if len(args) != 2 {
		fmt.Printf("Missing arguments : channels_sample transfer <channel_hash> <message_id>\n")
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

	message, err := channel.GetIncomingMessage(ctx, messageID)
	if err != nil {
		fmt.Printf("Failed to get message : %s\n", err)
		return
	}

	wrap, err := protocols.Unwrap(message.Payload())
	if err != nil {
		fmt.Printf("Failed to unwrap message : %s\n", err)
		return
	}

	request, ok := wrap.Message.(*invoices.TransferRequest)
	if !ok {
		fmt.Printf("Message is not a transfer request\n")
		return
	}

	if err := spClient.Connect(ctx); err != nil {
		fmt.Printf("Failed to connect to spynode : %s\n", err)
		return
	}
	defer spClient.Close(ctx)

	if _, err := client.Wallet.FundTx(ctx, *channelHash, request.Tx, request.Fees); err != nil {
		fmt.Printf("Failed to fund transfer : %s\n", err)
		return
	}

	if err := client.Wallet.SignTx(ctx, *channelHash, request.Tx); err != nil {
		fmt.Printf("Failed to sign transfer : %s\n", err)
		return
	}

	transfer := &invoices.Transfer{
		Tx: request.Tx,
	}

	if _, err := channel.SendMessage(ctx, transfer, &messageID); err != nil {
		fmt.Printf("Failed to send transfer : %s\n", err)
		return
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

	price := uint64(1000)
	purchaseOrder := &invoices.PurchaseOrder{
		Items: invoices.InvoiceItems{
			{
				ID: bitcoin.Hex("standard"),
				Price: invoices.Price{
					Quantity: &price,
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
	fmt.Printf("  Public Key : %s\n", channel.Key().PublicKey().String())

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
	userIdentity := relationships.Identity{
		Name: &userName,
	}

	initiation := &relationships.Initiation{
		Configuration: relationships.ChannelConfiguration{
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
		return
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

func display(ctx context.Context, protocols *channels.Protocols, sampleClient *sample_client.Client,
	args ...string) {

	if len(args) != 3 {
		fmt.Printf("Missing arguments : channels_sample display <channel_hash> <in or out> <message_id>\n")
		return
	}

	channelHash, err := bitcoin.NewHash32FromStr(args[0])
	if err != nil {
		fmt.Printf("Invalid channel hash : %s\n", err)
		return
	}

	inOrOut := args[1]

	messageID, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		fmt.Printf("Invalid message ID : %s\n", err)
		return
	}

	channel, err := sampleClient.ChannelsClient.GetChannelByHash(*channelHash)
	if err != nil {
		fmt.Printf("Failed to get channel : %s\n", err)
		return
	}

	var msg *client.Message
	if inOrOut == "in" {
		msg, err = channel.GetIncomingMessage(ctx, messageID)
		if err != nil {
			fmt.Printf("Failed to get message : %s\n", err)
			return
		}
	} else {
		msg, err = channel.GetOutgoingMessage(ctx, messageID)
		if err != nil {
			fmt.Printf("Failed to get message : %s\n", err)
			return
		}
	}

	wrap, err := protocols.Unwrap(msg.Payload())
	if err != nil {
		fmt.Printf("Failed to unwrap message : %s\n", err)
		return
	}

	js, err := json.MarshalIndent(wrap, "", "  ")
	if err != nil {
		fmt.Printf("Failed to marshal message : %s\n", err)
		return
	}

	fmt.Printf("%s\n", js)

	if transfer, ok := wrap.Message.(*invoices.Transfer); ok {
		if err := sampleClient.Wallet.VerifyFee(ctx, channel.Hash(), transfer.Tx,
			channels.DefaultFeeRequirements); err != nil {
			fmt.Printf("Failed to verify fee : %s\n", err)
		} else {
			fmt.Printf("\nVerified fee\n")
		}

		fmt.Printf(transfer.Tx.StringWithAddresses(bitcoin.MainNet))
	}
}

func list(ctx context.Context, protocols *channels.Protocols, client *sample_client.Client,
	args ...string) {

	msgs, err := client.ChannelsClient.GetUnprocessedMessages(ctx)
	if err != nil {
		fmt.Printf("Failed to get unprocessed messages : %s\n", err)
		return
	}

	fmt.Printf("%d messages:\n", len(msgs))

	for _, msg := range msgs {
		wrap, err := protocols.Unwrap(msg.Message.Payload())
		if err != nil {
			fmt.Printf("Failed to unwrap message : %s\n", err)
			continue
		}

		js, err := json.MarshalIndent(wrap, "", "  ")
		if err != nil {
			fmt.Printf("Failed to marshal message : %s\n", err)
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
		return
	}

	var wait sync.WaitGroup
	var stopper threads.StopCombiner
	spynodeErrors := make(chan error, 1)
	spyNodeClient.SetListenerErrorChannel(&spynodeErrors)

	if err := spyNodeClient.Connect(ctx); err != nil {
		logger.Error(ctx, "Failed to connect to spynode : %s", err)
		return
	}

	clientThread := threads.NewThread("Client", client.Run)
	clientThread.SetWait(&wait)
	clientComplete := clientThread.GetCompleteChannel()
	stopper.Add(clientThread)

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)

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
}
