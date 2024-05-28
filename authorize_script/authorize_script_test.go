package authorize_script

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"testing"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
)

func Test_Basic(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)

	testProtocolID := []byte("TEST")
	testData := make([]byte, 25)
	rand.Read(testData)
	testDataItem := bitcoin.NewPushDataScriptItem(testData)
	authPayload := envelope.Data{
		ProtocolIDs: envelope.ProtocolIDs{testProtocolID},
		Payload:     bitcoin.ScriptItems{testDataItem},
	}

	authScript, err := AuthorizeP2PK(authPayload, key)
	if err != nil {
		t.Fatalf("Failed to sign payload : %s", err)
	}

	payload, err := authScript.Wrap(authPayload)
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

	if !bytes.Equal(readPayload.ProtocolIDs[0], ProtocolID) {
		t.Fatalf("Wrong first protocol ID : got %x, want %x", readPayload.ProtocolIDs[0],
			ProtocolID)
	}

	if !bytes.Equal(readPayload.ProtocolIDs[1], testProtocolID) {
		t.Fatalf("Wrong second protocol ID : got %x, want %x", readPayload.ProtocolIDs[0],
			testProtocolID)
	}

	authorized, authorizedPayload, err := ParseAuthorize(readPayload)
	if err != nil {
		t.Fatalf("Failed to parse signed message : %s", err)
	}

	js, _ := json.MarshalIndent(authorized, "", "  ")
	t.Logf("Message : %s", js)

	if err := authorized.Verify(); err != nil {
		t.Errorf("Message script did not verify : %s", err)
	} else {
		t.Logf("Message script verified")
	}

	if len(authorizedPayload.ProtocolIDs) != 1 {
		t.Fatalf("Wrong protocol id count : got %d, want %d", len(authorizedPayload.ProtocolIDs), 1)
	}

	if !bytes.Equal(authorizedPayload.ProtocolIDs[0], testProtocolID) {
		t.Errorf("Wrong protocol id : got 0x%x, want 0x%x", authorizedPayload.ProtocolIDs[0],
			authorizedPayload)
	}

	if len(authorizedPayload.Payload) != 1 {
		t.Fatalf("Wrong payload count : got %d, want %d", len(authorizedPayload.Payload), 1)
	}

	if authorizedPayload.Payload[0].Type != bitcoin.ScriptItemTypePushData {
		t.Fatalf("Wrong payload type : got %d, want %d", authorizedPayload.Payload[0].Type,
			bitcoin.ScriptItemTypePushData)
	}

	if !bytes.Equal(authorizedPayload.Payload[0].Data, testData) {
		t.Errorf("Wrong protocol id : got 0x%x, want 0x%x", authorizedPayload.Payload[0].Data,
			testData)
	}
}
