package peer_channels

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/tokenized/channels"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/go-test/deep"
	"github.com/google/uuid"
)

func Test_PeerChannels_CalculatePeerChannelsServiceChannelID(t *testing.T) {

	tests := []struct {
		publicKey string
		channelID string
	}{
		{
			publicKey: "02b11cc88dbb41d61aa4eadbea40ce45085ed67725a202bcf457c764a0ede21a38",
			channelID: "878b8fca-10c6-eddf-87fd-dd8afecc4e2c",
		},
		{
			publicKey: "038cb9383fd651956b78d1b8015017f25391f4f65e92ba2fe37ee5d83ffa9da59e",
			channelID: "99325aef-7898-47cf-1407-0bdb45cfda7f",
		},
		{
			publicKey: "0295ab61b205cf5b3e81b8c8fd31fff807029d1826b650dcb2579cbb14a6eaf84a",
			channelID: "0e9e83f2-ade1-90ff-65a3-e4e0dbb62636",
		},
	}

	for _, test := range tests {
		publicKey, err := bitcoin.PublicKeyFromStr(test.publicKey)
		if err != nil {
			t.Fatalf("Failed to parse public key : %s", err)
		}

		channelID, err := uuid.Parse(test.channelID)
		if err != nil {
			t.Fatalf("Failed to parse channel id : %s", err)
		}

		calcChannelID := CalculatePeerChannelsServiceChannelID(publicKey)
		t.Logf("Channel ID : %s", calcChannelID)

		if calcChannelID != channelID.String() {
			t.Errorf("Wrong calculated id : got %s, want %s", calcChannelID, channelID)
		}
	}
}

func Test_PeerChannels_Create(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)

	msg := &CreateChannel{
		Type: ChannelTypePublic,
	}

	payload, err := msg.Write()
	if err != nil {
		t.Fatalf("Failed to write peer channels : %s", err)
	}

	signature, err := channels.Sign(payload, key, nil, true)
	if err != nil {
		t.Fatalf("Failed to sign payload : %s", err)
	}

	signedPayload, err := signature.Wrap(payload)
	if err != nil {
		t.Fatalf("Failed to create signed payload : %s", err)
	}

	envelopeScriptItems := envelopeV1.Wrap(signedPayload)

	script, err := envelopeScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create script : %s", err)
	}

	t.Logf("Script (%d bytes) : %s", len(script), script)

	readPayload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		t.Fatalf("Failed to parse script : %s", err)
	}

	signed, signedPayload, err := channels.ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, _, err := Parse(signedPayload)
	if err != nil {
		t.Fatalf("Failed to read peer channels : %s", err)
	}

	js, _ := json.MarshalIndent(readMsg, "", "  ")
	t.Logf("PeerChannel message : %s", js)

	if _, ok := readMsg.(*CreateChannel); !ok {
		t.Errorf("Wrong message type")
	}

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("Unmarshalled value not equal : %v", deep.Equal(readMsg, msg))
	}
}

func Test_PeerChannels_Delete(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)

	msg := &DeleteChannel{
		ID: uuid.New().String(),
	}

	payload, err := msg.Write()
	if err != nil {
		t.Fatalf("Failed to write peer channels : %s", err)
	}

	signature, err := channels.Sign(payload, key, nil, true)
	if err != nil {
		t.Fatalf("Failed to sign payload : %s", err)
	}

	signedPayload, err := signature.Wrap(payload)
	if err != nil {
		t.Fatalf("Failed to create signed payload : %s", err)
	}

	envelopeScriptItems := envelopeV1.Wrap(signedPayload)
	script, err := envelopeScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create script : %s", err)
	}

	t.Logf("Script (%d bytes) : %s", len(script), script)

	readPayload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		t.Fatalf("Failed to parse script : %s", err)
	}

	signed, signedPayload, err := channels.ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, _, err := Parse(signedPayload)
	if err != nil {
		t.Fatalf("Failed to read peer channels : %s", err)
	}

	js, _ := json.MarshalIndent(readMsg, "", "  ")
	t.Logf("PeerChannel message : %s", js)

	if _, ok := readMsg.(*DeleteChannel); !ok {
		t.Errorf("Wrong message type")
	}

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("Unmarshalled value not equal : %v", deep.Equal(readMsg, msg))
	}
}
