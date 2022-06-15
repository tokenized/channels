package sample_client

import (
	"context"
	"testing"

	"github.com/tokenized/channels/client"
	"github.com/tokenized/channels/wallet"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"
)

func Test_Keys(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")

	for i := 0; i < 10; i++ {
		store := storage.NewMockStorage()
		protocols := client.BuildChannelsProtocols()
		peerChannelsFactory := peer_channels.NewFactory()
		merkleProofVerifier := wallet.NewMockMerkleProofVerifier()
		feeQuoter := wallet.NewMockFeeQuoter()
		contextID := wallet.RandomHash()

		key, err := bitcoin.GenerateKey(bitcoin.MainNet)
		if err != nil {
			t.Fatalf("Failed to generate key : %s", err)
		}

		client := NewClient(key, store, protocols, peerChannelsFactory, merkleProofVerifier,
			feeQuoter, nil)

		tx, _, err := client.Wallet.CreateBitcoinReceive(ctx, contextID, 10000)
		if err != nil {
			t.Fatalf("Failed to create receive : %s", err)
		}

		t.Logf("Bitcoin Receive : \n%s\n", tx.Tx.StringWithAddresses(bitcoin.MainNet))

		if err := client.Save(ctx); err != nil {
			t.Fatalf("Failed to save client : %s", err)
		}

		loadedClient := NewClient(key, store, protocols, peerChannelsFactory, merkleProofVerifier,
			feeQuoter, nil)

		if err := loadedClient.Load(ctx); err != nil {
			t.Fatalf("Failed to load client : %s", err)
		}

		keys, err := loadedClient.Wallet.GetKeys(ctx, contextID)
		if err != nil {
			t.Fatalf("Failed to get keys : %s", err)
		}

		t.Logf("Found %d keys\n", len(keys))
		for _, key := range keys {
			ra, _ := key.Key.RawAddress()
			t.Logf("  Address : %s\n", bitcoin.NewAddressFromRawAddress(ra, bitcoin.MainNet))
		}
	}
}
