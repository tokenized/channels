package channels

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/go-test/deep"
	"github.com/google/uuid"
)

func Test_PeerChannels_Create(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)

	msg := &PeerChannelsCreateChannel{
		Type: PeerChannelTypePublic,
	}

	payload, err := msg.Write()
	if err != nil {
		t.Fatalf("Failed to write peer channels : %s", err)
	}

	signature, err := Sign(payload, key, nil, true)
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

	signed, signedPayload, err := ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, err := ParsePeerChannels(signedPayload)
	if err != nil {
		t.Fatalf("Failed to read peer channels : %s", err)
	}

	js, _ := json.MarshalIndent(readMsg, "", "  ")
	t.Logf("PeerChannel message : %s", js)

	if _, ok := readMsg.(*PeerChannelsCreateChannel); !ok {
		t.Errorf("Wrong message type")
	}

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("Unmarshalled value not equal : %v", deep.Equal(readMsg, msg))
	}
}

func Test_PeerChannels_Delete(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)

	msg := &PeerChannelsDeleteChannel{
		ID: uuid.New(),
	}

	payload, err := msg.Write()
	if err != nil {
		t.Fatalf("Failed to write peer channels : %s", err)
	}

	signature, err := Sign(payload, key, nil, true)
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

	signed, signedPayload, err := ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, err := ParsePeerChannels(signedPayload)
	if err != nil {
		t.Fatalf("Failed to read peer channels : %s", err)
	}

	js, _ := json.MarshalIndent(readMsg, "", "  ")
	t.Logf("PeerChannel message : %s", js)

	if _, ok := readMsg.(*PeerChannelsDeleteChannel); !ok {
		t.Errorf("Wrong message type")
	}

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("Unmarshalled value not equal : %v", deep.Equal(readMsg, msg))
	}
}
