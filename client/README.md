# Channels Client

Channels Client is designed to facilitate full implementations of wallets and services that use the Channels protocols. It needs to be paired with a wallet for payments, which are an integral part of Channels.


## Setup

### Create a new client instance

```
peerChannelsBaseURL := "http://..."
peerChannelsFactory := peer_channels.NewFactory()
account := Account{
	BaseURL: peerChannelsBaseURL,
	ID:      accountID,
	Token:   accountToken,
}

identity := Identity{
	Name: name,
	// Include any of the other fields as wanted.
}

client := NewClient(account, identity, peerChannelsFactory)
```

### Create a public relationship initiation channel

```
peerChannelClient, err := peerChannelsFactory.NewClient(peerChannelsBaseURL)
if err != nil {
	return errors.Wrap(err, "peer client")
}
```
