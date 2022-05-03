package channels

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/go-test/deep"
)

func Test_PeerChannels_Create(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)

	msg := &CreateChannel{
		Type: PeerChannelTypePublic,
	}

	protocolIDs, scriptItems, err := WritePeerChannel(msg)
	if err != nil {
		t.Fatalf("Failed to write peer channels : %s", err)
	}

	signedProtocolIDs, signedScriptItems, err := Sign(protocolIDs, scriptItems, key, nil, true)

	envelopeScriptItems := envelopeV1.Wrap(signedProtocolIDs, signedScriptItems)

	script, err := envelopeScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create script : %s", err)
	}

	t.Logf("Script (%d bytes) : %s", len(script), script)

	readProtocolIDs, readPayload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		t.Fatalf("Failed to parse script : %s", err)
	}

	signed, signedProtocolIDs, signedPayload, err := ParseSigned(readProtocolIDs, readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, err := ParsePeerChannel(signedProtocolIDs, signedPayload)
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
		ID: uuid.New(),
	}

	protocolIDs, scriptItems, err := WritePeerChannel(msg)
	if err != nil {
		t.Fatalf("Failed to write peer channels : %s", err)
	}

	signedProtocolIDs, signedScriptItems, err := Sign(protocolIDs, scriptItems, key, nil, true)

	envelopeScriptItems := envelopeV1.Wrap(signedProtocolIDs, signedScriptItems)

	script, err := envelopeScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create script : %s", err)
	}

	t.Logf("Script (%d bytes) : %s", len(script), script)

	readProtocolIDs, readPayload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		t.Fatalf("Failed to parse script : %s", err)
	}

	signed, signedProtocolIDs, signedPayload, err := ParseSigned(readProtocolIDs, readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, err := ParsePeerChannel(signedProtocolIDs, signedPayload)
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
