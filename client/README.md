# Channels Client

Channels Client is designed to facilitate full implementations of wallets and services that use the Channels protocols. It needs to be paired with a wallet for payments, which are an integral part of Channels.


## Setup

### Create a new client instance

```
ctx := logger.ContextWithLogger(context.Background(), true, true, "")

// Create object that implements the `storage.StreamReadWriter` interface.
store := storage.NewFilesystemStorage(storeConfig)

peerChannelsBaseURL := "http://..."
peerChannelsFactory := peer_channels.NewFactory()

// Get private key from config. It must remain the same for this client instance.
baseKey := getBaseKeyFromConfig()

// Create a peer channels account and put that information in a config to be used here for the
// client to use.
account := Account{
	BaseURL: peerChannelsBaseURL,
	ID:      getAccountIDFromConfig(),
	Token:   getAccountTokenFromConfig(),
}

client := NewClient(baseKey, account, store, peerChannelsFactory)
```

### Create a public relationship initiation channel

Only do this on first startup. It is retained by `client.Save`.

```
// Create publicly writable peer channel and setup client channel on top to process incoming messages.
initiationChannel, err := client.CreateRelationshipInitiationChannel(ctx)
if err != nil {
	return errors.Wrap(err, "create initiation channel")
}

// Get peer channels data to provide to others so they can initiate a relationship with you.
initiationPeerChannels := initiationChannel.IncomingPeerChannels()

js, _ := json.MarshalIndent(initiationPeerChannels, "", "  ")
println(js)
```
