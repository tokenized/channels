package negotiation

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/tokenized/channels"
	channelsExpandedTx "github.com/tokenized/channels/expanded_tx"
	"github.com/tokenized/pkg/expanded_tx"
	"github.com/tokenized/pkg/fees"
	"github.com/tokenized/pkg/wire"

	"github.com/google/uuid"
)

func Test_Serialize(t *testing.T) {
	protocols := channels.NewProtocols(
		channels.NewFeeRequirementsProtocol(),
		channels.NewStringIDProtocol(),
		channels.NewNoteProtocol(),
		channels.NewReplyToProtocol(),
		channelsExpandedTx.NewProtocol(),
	)

	threadID1 := uuid.New().String()
	note1 := "This is a note"
	handle1 := "user@handle.com"

	tx := wire.NewMsgTx(1)

	randScript := make([]byte, 100)
	rand.Read(randScript)
	tx.AddTxOut(wire.NewTxOut(1000, randScript))

	tests := []struct {
		ntx Transaction
	}{
		{
			ntx: Transaction{
				ThreadID: &threadID1,
				Fees:     fees.DefaultFeeRequirements,
				ReplyTo: &channels.ReplyTo{
					Handle: &handle1,
				},
				Note: &note1,
				Tx: &expanded_tx.ExpandedTx{
					Tx: tx,
				},
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			js, _ := json.MarshalIndent(tt.ntx, "", "  ")
			t.Logf("Negotiation tx : %s", js)

			script, err := tt.ntx.Wrap()
			if err != nil {
				t.Fatalf("Failed to wrap negotiation tx : %s", err)
			}

			t.Logf("Script : %s", script)

			msg, wrappers, err := protocols.Parse(script)
			if err != nil {
				t.Fatalf("Failed to parse negotiation tx : %s", err)
			}

			ntx, extra, err := CompileTransaction(msg, wrappers)
			if err != nil {
				t.Fatalf("Failed to compile negotiation tx : %s", err)
			}

			js, _ = json.MarshalIndent(ntx, "", "  ")
			t.Logf("Read Negotiation tx : %s", js)

			if len(extra) != 0 {
				t.Errorf("Extra wrappers found")
			}
		})
	}
}
