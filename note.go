package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

const (
	NoteVersion = uint8(0)
)

var (
	ProtocolIDNote = envelope.ProtocolID("NOTE") // Protocol ID for notes

	ErrInvalidNote = errors.New("Invalid Note")
)

type NoteProtocol struct{}

func NewNoteProtocol() *NoteProtocol {
	return &NoteProtocol{}
}

func (*NoteProtocol) ProtocolID() envelope.ProtocolID {
	return ProtocolIDNote
}

func (*NoteProtocol) Parse(payload envelope.Data) (Message, envelope.Data, error) {
	return ParseNote(payload)
}

func (*NoteProtocol) ResponseCodeToString(code uint32) string {
	return "parse"
}

type Note struct {
	Note string `bsor:"-" json:"note"`
}

func NewNote(note string) *Note {
	return &Note{Note: note}
}

func (*Note) IsWrapperType() {}

func (*Note) ProtocolID() envelope.ProtocolID {
	return ProtocolIDNote
}

// WrapNote wraps the payload with the note and returns the new payload containing the note.
func WrapNote(payload envelope.Data, note string) (envelope.Data, error) {
	id := &Note{
		Note: note,
	}

	return id.Wrap(payload)
}

func (m *Note) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(NoteVersion))}

	// Message
	scriptItems = append(scriptItems, bitcoin.NewPushDataScriptItem([]byte(m.Note)))

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDNote}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func ParseNote(payload envelope.Data) (*Note, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 || !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDNote) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 3 {
		return nil, payload, errors.Wrapf(ErrInvalidMessage, "not enough note push ops: %d",
			len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion,
			fmt.Sprintf("note: %d", version))
	}

	if payload.Payload[1].Type != bitcoin.ScriptItemTypePushData {
		return nil, payload, errors.New("Not Push Data")
	}

	result := &Note{
		Note: string(payload.Payload[1].Data),
	}

	payload.Payload = payload.Payload[2:]
	return result, payload, nil
}
