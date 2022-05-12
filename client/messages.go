package client

import (
	"encoding/binary"
	"io"
	"sync"

	"github.com/tokenized/channels"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

const (
	messagesVersion = uint8(0)

	messagesPerFile = 100
)

type Message struct {
	id                 uint64
	payload            bitcoin.Script
	timestamp          channels.Timestamp
	isAwaitingResponse bool
	isProcessed        bool

	response bitcoin.Script // base of message suggested as response

	lock sync.Mutex
}

type Messages []*Message

type ChannelMessage struct {
	Message Message
	Channel *Channel
}

type ChannelMessages []*ChannelMessage

func (m *Message) ID() uint64 {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.id
}

func (m *Message) Payload() bitcoin.Script {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.payload
}

func (m *Message) SetPayload(script bitcoin.Script) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.payload = script
}

func (m *Message) Timestamp() channels.Timestamp {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.timestamp
}

func (m *Message) SetNow() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.timestamp = channels.Now()
}

func (m *Message) IsAwaitingResponse() bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.isAwaitingResponse
}

func (m *Message) SetIsAwaitingResponse() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.isAwaitingResponse = true
}

func (m *Message) ClearIsAwaitingResponse() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.isAwaitingResponse = false
}

func (m *Message) IsProcessed() bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.isProcessed
}

func (m *Message) setIsProcessed() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.isProcessed = true
}

func (m *Message) clearIsProcessed() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.isProcessed = false
}

func (m *Message) Response() bitcoin.Script {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.response
}

func (m *Message) SetResponse(script bitcoin.Script) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.response = script
}

func (m *Message) Reject(reject *channels.Reject) error {
	reject.MessageID = m.ID()

	payload, err := reject.Write()
	if err != nil {
		return errors.Wrap(err, "write")
	}

	scriptItems := envelopeV1.Wrap(payload)
	script, err := scriptItems.Script()
	if err != nil {
		return errors.Wrap(err, "script")
	}

	m.SetResponse(script)
	return nil
}

func (m *Message) Serialize(w io.Writer) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	payloadSize := uint32(len(m.payload))
	if err := binary.Write(w, endian, payloadSize); err != nil {
		return errors.Wrap(err, "payload size")
	}

	if _, err := w.Write(m.payload); err != nil {
		return errors.Wrap(err, "payload")
	}

	if err := binary.Write(w, endian, m.timestamp); err != nil {
		return errors.Wrap(err, "timestamp")
	}

	if err := binary.Write(w, endian, m.isAwaitingResponse); err != nil {
		return errors.Wrap(err, "isAwaitingResponse")
	}

	if err := binary.Write(w, endian, m.isProcessed); err != nil {
		return errors.Wrap(err, "isProcessed")
	}

	if len(m.response) == 0 {
		if err := binary.Write(w, endian, false); err != nil {
			return errors.Wrap(err, "has response")
		}
	} else {
		responseSize := uint32(len(m.response))
		if err := binary.Write(w, endian, responseSize); err != nil {
			return errors.Wrap(err, "response size")
		}

		if _, err := w.Write(m.response); err != nil {
			return errors.Wrap(err, "response")
		}
	}

	return nil
}

func (m *Message) Deserialize(r io.Reader) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	var payloadSize uint32
	if err := binary.Read(r, endian, &payloadSize); err != nil {
		return errors.Wrap(err, "payload size")
	}

	m.payload = make(bitcoin.Script, payloadSize)
	if _, err := io.ReadFull(r, m.payload); err != nil {
		return errors.Wrap(err, "payload")
	}

	if err := binary.Read(r, endian, &m.timestamp); err != nil {
		return errors.Wrap(err, "timestamp")
	}

	if err := binary.Read(r, endian, &m.isAwaitingResponse); err != nil {
		return errors.Wrap(err, "isAwaitingResponse")
	}

	if err := binary.Read(r, endian, &m.isProcessed); err != nil {
		return errors.Wrap(err, "isProcessed")
	}

	var hasResponse bool
	if err := binary.Read(r, endian, &hasResponse); err != nil {
		return errors.Wrap(err, "has response")
	}

	if hasResponse {
		var responseSize uint32
		if err := binary.Read(r, endian, &responseSize); err != nil {
			return errors.Wrap(err, "response size")
		}

		m.response = make(bitcoin.Script, responseSize)
		if _, err := io.ReadFull(r, m.response); err != nil {
			return errors.Wrap(err, "response")
		}
	}

	return nil
}

func (m Messages) Serialize(w io.Writer) error {
	if err := binary.Write(w, endian, messagesVersion); err != nil {
		return errors.Wrap(err, "version")
	}

	count := uint32(len(m))
	if err := binary.Write(w, endian, count); err != nil {
		return errors.Wrap(err, "count")
	}

	for i, message := range m {
		if err := message.Serialize(w); err != nil {
			return errors.Wrapf(err, "message %d", i)
		}
	}

	return nil
}

func (m *Messages) Deserialize(r io.Reader) error {
	var version uint8
	if err := binary.Read(r, endian, &version); err != nil {
		return errors.Wrap(err, "version")
	}
	if version != 0 {
		return errors.New("Unsupported version")
	}

	var count uint32
	if err := binary.Read(r, endian, &count); err != nil {
		return errors.Wrap(err, "count")
	}

	*m = make(Messages, count)
	for i := range *m {
		message := &Message{}
		if err := message.Deserialize(r); err != nil {
			return errors.Wrapf(err, "message %d", i)
		}
		(*m)[i] = message
	}

	return nil
}
