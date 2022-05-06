package channels

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"testing"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

func Test_SignedMessage_WithKey(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)

	testProtocolID := []byte("TEST")
	testData := make([]byte, 25)
	rand.Read(testData)
	testDataItem := bitcoin.NewPushDataScriptItem(testData)

	signature, err := Sign(envelope.Data{envelope.ProtocolIDs{testProtocolID},
		bitcoin.ScriptItems{testDataItem}}, key, nil, true)
	if err != nil {
		t.Fatalf("Failed to sign payload : %s", err)
	}

	payload, err := signature.Wrap(envelope.Data{envelope.ProtocolIDs{testProtocolID},
		bitcoin.ScriptItems{testDataItem}})
	if err != nil {
		t.Fatalf("Failed to create signed payload : %s", err)
	}

	envelopeScriptItems := envelopeV1.Wrap(payload)
	script, err := envelopeScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create script : %s", err)
	}

	t.Logf("Script : %s", script)

	buf := bytes.NewReader(script)
	readPayload, err := envelopeV1.Parse(buf)
	if err != nil {
		t.Fatalf("Failed to parse envelope : %s", err)
	}

	if len(readPayload.ProtocolIDs) != 2 {
		t.Fatalf("Wrong protocol ID count : got %d, want %d", len(readPayload.ProtocolIDs), 2)
	}

	if !bytes.Equal(readPayload.ProtocolIDs[0], ProtocolIDSignedMessages) {
		t.Fatalf("Wrong first protocol ID : got %x, want %x", readPayload.ProtocolIDs[0],
			ProtocolIDSignedMessages)
	}

	if !bytes.Equal(readPayload.ProtocolIDs[1], testProtocolID) {
		t.Fatalf("Wrong second protocol ID : got %x, want %x", readPayload.ProtocolIDs[0],
			testProtocolID)
	}

	signed, signedPayload, err := ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to parse signed message : %s", err)
	}

	js, _ := json.MarshalIndent(signed, "", "  ")
	t.Logf("Message : %s", js)

	if err := signed.Verify(); err != nil {
		t.Errorf("Message signature did not verify : %s", err)
	} else {
		t.Logf("Message signature verified")
	}

	if len(signedPayload.ProtocolIDs) != 1 {
		t.Fatalf("Wrong protocol id count : got %d, want %d", len(signedPayload.ProtocolIDs), 1)
	}

	if !bytes.Equal(signedPayload.ProtocolIDs[0], testProtocolID) {
		t.Errorf("Wrong protocol id : got 0x%x, want 0x%x", signedPayload.ProtocolIDs[0],
			testProtocolID)
	}

	if len(signedPayload.Payload) != 1 {
		t.Fatalf("Wrong payload count : got %d, want %d", len(signedPayload.Payload), 1)
	}

	if signedPayload.Payload[0].Type != bitcoin.ScriptItemTypePushData {
		t.Fatalf("Wrong payload type : got %d, want %d", signedPayload.Payload[0].Type,
			bitcoin.ScriptItemTypePushData)
	}

	if !bytes.Equal(signedPayload.Payload[0].Data, testData) {
		t.Errorf("Wrong protocol id : got 0x%x, want 0x%x", signedPayload.Payload[0].Data, testData)
	}
}

func Test_SignedMessage_WithoutKey(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)
	publicKey := key.PublicKey()

	testProtocolID := []byte("TEST")
	testData := make([]byte, 25)
	rand.Read(testData)
	testDataItem := bitcoin.NewPushDataScriptItem(testData)

	signature, err := Sign(envelope.Data{envelope.ProtocolIDs{testProtocolID},
		bitcoin.ScriptItems{testDataItem}}, key, nil, false)
	if err != nil {
		t.Fatalf("Failed to sign payload : %s", err)
	}

	payload, err := signature.Wrap(envelope.Data{envelope.ProtocolIDs{testProtocolID},
		bitcoin.ScriptItems{testDataItem}})
	if err != nil {
		t.Fatalf("Failed to create signed payload : %s", err)
	}

	envelopeScriptItems := envelopeV1.Wrap(payload)
	script, _ := envelopeScriptItems.Script()
	t.Logf("Script : %s", script)

	buf := bytes.NewReader(script)
	readPayload, err := envelopeV1.Parse(buf)
	if err != nil {
		t.Fatalf("Failed to parse envelope : %s", err)
	}

	if len(readPayload.ProtocolIDs) != 2 {
		t.Fatalf("Wrong protocol ID count : got %d, want %d", len(readPayload.ProtocolIDs), 2)
	}

	if !bytes.Equal(readPayload.ProtocolIDs[0], ProtocolIDSignedMessages) {
		t.Fatalf("Wrong first protocol ID : got %x, want %x", readPayload.ProtocolIDs[0],
			ProtocolIDSignedMessages)
	}

	if !bytes.Equal(readPayload.ProtocolIDs[1], testProtocolID) {
		t.Fatalf("Wrong second protocol ID : got %x, want %x", readPayload.ProtocolIDs[0],
			testProtocolID)
	}

	signed, signedPayload, err := ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to parse signed message : %s", err)
	}

	js, _ := json.MarshalIndent(signed, "", "  ")
	t.Logf("Message : %s", js)

	if err := signed.Verify(); err == nil {
		t.Errorf("Message signature should not verify without key")
	} else if errors.Cause(err) != ErrPublicKeyMissing {
		t.Errorf("Message signature verify error is wrong : got %s, want %s", err,
			ErrPublicKeyMissing)
	} else {
		t.Logf("Message signature verify correctly failed without key : %s", err)
	}

	signed.SetPublicKey(&publicKey)

	if err := signed.Verify(); err != nil {
		t.Errorf("Message signature did not verify : %s", err)
	} else {
		t.Logf("Message signature verified")
	}

	if len(signedPayload.ProtocolIDs) != 1 {
		t.Fatalf("Wrong protocol id count : got %d, want %d", len(signedPayload.ProtocolIDs), 1)
	}

	if !bytes.Equal(signedPayload.ProtocolIDs[0], testProtocolID) {
		t.Errorf("Wrong protocol id : got 0x%x, want 0x%x", signedPayload.ProtocolIDs[0],
			testProtocolID)
	}

	if len(signedPayload.Payload) != 1 {
		t.Fatalf("Wrong payload count : got %d, want %d", len(signedPayload.Payload), 1)
	}

	if signedPayload.Payload[0].Type != bitcoin.ScriptItemTypePushData {
		t.Fatalf("Wrong payload type : got %d, want %d", signedPayload.Payload[0].Type,
			bitcoin.ScriptItemTypePushData)
	}

	if !bytes.Equal(signedPayload.Payload[0].Data, testData) {
		t.Errorf("Wrong protocol id : got 0x%x, want 0x%x", signedPayload.Payload[0].Data, testData)
	}
}

func Test_SignedMessage_WithHash(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)
	basePublicKey := key.PublicKey()
	t.Logf("Base public key : %s", basePublicKey)

	testProtocolID := []byte("TEST")
	testData := make([]byte, 25)
	rand.Read(testData)
	testDataItem := bitcoin.NewPushDataScriptItem(testData)
	var derivationHash bitcoin.Hash32
	rand.Read(derivationHash[:])
	t.Logf("Derivation hash : %s", derivationHash)

	derivedPublicKey, err := basePublicKey.AddHash(derivationHash)
	if err != nil {
		t.Fatalf("Failed to derive public key : %s", err)
	}
	t.Logf("Derived public key : %s", derivedPublicKey)

	signature, err := Sign(envelope.Data{envelope.ProtocolIDs{testProtocolID},
		bitcoin.ScriptItems{testDataItem}}, key, &derivationHash, false)
	if err != nil {
		t.Fatalf("Failed to sign payload : %s", err)
	}

	payload, err := signature.Wrap(envelope.Data{envelope.ProtocolIDs{testProtocolID},
		bitcoin.ScriptItems{testDataItem}})
	if err != nil {
		t.Fatalf("Failed to create signed payload : %s", err)
	}

	envelopeScriptItems := envelopeV1.Wrap(payload)
	script, err := envelopeScriptItems.Script()

	t.Logf("Script : %s", script)

	buf := bytes.NewReader(script)
	readPayload, err := envelopeV1.Parse(buf)
	if err != nil {
		t.Fatalf("Failed to parse envelope : %s", err)
	}

	if len(readPayload.ProtocolIDs) != 2 {
		t.Fatalf("Wrong protocol ID count : got %d, want %d", len(readPayload.ProtocolIDs), 2)
	}

	if !bytes.Equal(readPayload.ProtocolIDs[0], ProtocolIDSignedMessages) {
		t.Fatalf("Wrong first protocol ID : got %x, want %x", readPayload.ProtocolIDs[0],
			ProtocolIDSignedMessages)
	}

	if !bytes.Equal(readPayload.ProtocolIDs[1], testProtocolID) {
		t.Fatalf("Wrong second protocol ID : got %x, want %x", readPayload.ProtocolIDs[0],
			testProtocolID)
	}

	signed, signedPayload, err := ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to parse signed message : %s", err)
	}

	js, _ := json.MarshalIndent(signed, "", "  ")
	t.Logf("Message : %s", js)

	if err := signed.Verify(); err == nil {
		t.Errorf("Message signature should not verify without key")
	} else if errors.Cause(err) != ErrPublicKeyMissing {
		t.Errorf("Message signature verify error is wrong : got %s, want %s", err,
			ErrPublicKeyMissing)
	} else {
		t.Logf("Message signature verify correctly failed without key : %s", err)
	}

	signed.SetPublicKey(&basePublicKey)

	readDerivedPublicKey, err := signed.GetPublicKey()
	if err != nil {
		t.Fatalf("Failed to get derived public key : %s", err)
	}
	t.Logf("Read derived public key : %s", readDerivedPublicKey)

	if !readDerivedPublicKey.Equal(derivedPublicKey) {
		t.Errorf("Derived public key does not match : got %s, want %s", readDerivedPublicKey,
			derivedPublicKey)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Message signature did not verify : %s", err)
	} else {
		t.Logf("Message signature verified")
	}

	if len(signedPayload.ProtocolIDs) != 1 {
		t.Fatalf("Wrong protocol id count : got %d, want %d", len(signedPayload.ProtocolIDs), 1)
	}

	if !bytes.Equal(signedPayload.ProtocolIDs[0], testProtocolID) {
		t.Errorf("Wrong protocol id : got 0x%x, want 0x%x", signedPayload.ProtocolIDs[0],
			testProtocolID)
	}

	if len(signedPayload.Payload) != 1 {
		t.Fatalf("Wrong payload count : got %d, want %d", len(signedPayload.Payload), 1)
	}

	if signedPayload.Payload[0].Type != bitcoin.ScriptItemTypePushData {
		t.Fatalf("Wrong payload type : got %d, want %d", signedPayload.Payload[0].Type,
			bitcoin.ScriptItemTypePushData)
	}

	if !bytes.Equal(signedPayload.Payload[0].Data, testData) {
		t.Errorf("Wrong protocol id : got 0x%x, want 0x%x", signedPayload.Payload[0].Data, testData)
	}
}
