package sample_client

import (
	"context"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/wallet"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	spyNodeClient "github.com/tokenized/spynode/pkg/client"

	"github.com/pkg/errors"
)

func (c *Client) HandleTx(ctx context.Context, tx *spyNodeClient.Tx) {
	if err := c.Wallet.AddTxWithoutContext(ctx, tx.Tx); err != nil {
		logger.Error(ctx, "Failed to add tx : %s", err)
	}

	txid := *tx.Tx.TxHash()
	c.applyTxState(ctx, txid, &tx.State)
}

func (c *Client) HandleTxUpdate(ctx context.Context, txUpdate *spyNodeClient.TxUpdate) {
	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("txid", txUpdate.TxID),
	}, "Received tx update")
	c.applyTxState(ctx, txUpdate.TxID, &txUpdate.State)
}

func (c *Client) applyTxState(ctx context.Context, txid bitcoin.Hash32,
	txState *spyNodeClient.TxState) {
	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("txid", txid))

	if txState.Safe {
		if err := c.Wallet.MarkTxSafe(ctx, txid); err != nil {
			if errors.Cause(err) != wallet.ErrUnknownTx {
				logger.Error(ctx, "Failed to mark wallet tx as safe : %s", err)
			}
		}
	}

	if txState.UnSafe {
		if err := c.Wallet.MarkTxUnsafe(ctx, txid); err != nil {
			if errors.Cause(err) != wallet.ErrUnknownTx {
				logger.Error(ctx, "Failed to mark wallet tx as unsafe : %s", err)
			}
		}
	}

	if txState.Cancelled {
		if err := c.Wallet.MarkTxCancelled(ctx, txid); err != nil {
			if errors.Cause(err) != wallet.ErrUnknownTx {
				logger.Error(ctx, "Failed to mark wallet tx as cancelled : %s", err)
			}
		}
	}

	if txState.MerkleProof != nil {
		merkleProof := txState.MerkleProof.ConvertToMerkleProof(txid)
		channelHashes, err := c.Wallet.AddMerkleProof(ctx, merkleProof)
		if err != nil {
			if errors.Cause(err) != wallet.ErrUnknownTx {
				logger.Error(ctx, "Failed to add merkle proof to wallet : %s", err)
			} else {
				logger.Info(ctx, "Unknown tx")
			}
		} else {
			logger.InfoWithFields(ctx, []logger.Field{
				logger.Stringer("block_hash", merkleProof.GetBlockHash()),
				logger.Int("channel_count", len(channelHashes)),
			}, "Added merkleproof to tx")

			// Send merkle proof to related channels
			for _, channelHash := range channelHashes {
				channel, err := c.ChannelsClient.GetChannelByHash(channelHash)
				if err != nil {
					logger.Error(ctx, "Failed to get channel : %s", err)
				} else if channel != nil {
					if _, err := channel.SendMessage(ctx, &channels.MerkleProof{merkleProof},
						nil); err != nil {
						logger.ErrorWithFields(ctx, []logger.Field{
							logger.Stringer("channel", channelHash),
						}, "Failed to send message to channel : %s", err)
					}
				}
			}
		}
	}
}

func (c *Client) HandleHeaders(ctx context.Context, headers *spyNodeClient.Headers) {
	if headers.RequestHeight <= 0 { // This is a newly created header.
		c.Wallet.SetBlockHeight(ctx, headers.StartHeight+uint32(len(headers.Headers)))
	}
}

func (c *Client) HandleInSync(ctx context.Context) {

}

func (c *Client) HandleMessage(ctx context.Context, payload spyNodeClient.MessagePayload) {
	switch msg := payload.(type) {
	case *spyNodeClient.AcceptRegister:
		logger.Info(ctx, "Spynode registration accepted")

		if c.nextSpyNodeMessageID == 0 || c.nextSpyNodeMessageID > msg.MessageCount+1 {
			logger.WarnWithFields(ctx, []logger.Field{
				logger.Uint64("next_message_id", c.nextSpyNodeMessageID),
				logger.Uint64("message_count", msg.MessageCount),
			}, "Resetting next message id")
			c.nextSpyNodeMessageID = 1 // first message is 1
		}

		if err := c.spyNodeClient.Ready(ctx, c.nextSpyNodeMessageID); err != nil {
			logger.Error(ctx, "Failed to notify spynode ready : %s", err)
		}

		logger.InfoWithFields(ctx, []logger.Field{
			logger.Uint64("next_message_id", c.nextSpyNodeMessageID),
		}, "Spynode client ready")
	}
}
