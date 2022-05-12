package client

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/storage"
)

func Test_Messages_Load(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	basePath := "test"

	tests := []struct {
		name         string
		total        int
		processCount int
	}{
		{
			name:         "0",
			total:        0,
			processCount: 0,
		},
		{
			name:         "1",
			total:        1,
			processCount: 1,
		},
		{
			name:         "10",
			total:        10,
			processCount: 10,
		},
		{
			name:         "50",
			total:        50,
			processCount: 50,
		},
		{
			name:         "99",
			total:        99,
			processCount: 99,
		},
		{
			name:         "100",
			total:        100,
			processCount: 100,
		},
		{
			name:         "101",
			total:        101,
			processCount: 101,
		},
		{
			name:         "199",
			total:        199,
			processCount: 199,
		},
		{
			name:         "200",
			total:        200,
			processCount: 200,
		},
		{
			name:         "201",
			total:        201,
			processCount: 201,
		},
		{
			name:         "250",
			total:        250,
			processCount: 10,
		},
		{
			name:         "250",
			total:        250,
			processCount: 110,
		},
		{
			name:         "350",
			total:        350,
			processCount: 110,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := storage.NewMockStorage()
			channel := newCommunicationChannel(store, basePath)

			messages := make([]bitcoin.Script, test.total)
			for i := range messages {
				script := make(bitcoin.Script, 25)
				rand.Read(script)

				message, err := channel.newMessageWithPayload(ctx, script)
				if err != nil {
					t.Fatalf("Failed to add message : %s", err)
				}
				messages[i] = script

				if i < test.processCount {
					channel.MarkMessageProcessed(ctx, message.ID())
				}
			}

			if err := channel.Save(ctx); err != nil {
				t.Fatalf("Failed to save channel : %s", err)
			}

			readChannel := newCommunicationChannel(store, basePath)
			if err := readChannel.Load(ctx); err != nil {
				t.Fatalf("Failed to load channel : %s", err)
			}

			if int(readChannel.messageCount) != test.total {
				t.Fatalf("Wrong message count : got %d, want %d", readChannel.messageCount,
					test.total)
			}

			t.Logf("Loaded %d messages", len(readChannel.messages))

			if len(readChannel.messages) < messagesPerFile &&
				len(readChannel.messages) < test.total {
				t.Errorf("Not enough messages loaded")
			}

			currentID := uint64(test.total - len(readChannel.messages))

			t.Logf("First loaded message : %d", currentID)

			if uint64(readChannel.loadedOffset) != currentID {
				t.Errorf("Wrong loaded offset : got %d, want %d", readChannel.loadedOffset,
					currentID)
			}

			if uint32(readChannel.loadedOffset) > readChannel.lowestUnprocessed {
				t.Errorf("Loaded offset higher than lowest unprocessed : loaded %d, unprocessed %d",
					readChannel.loadedOffset, readChannel.lowestUnprocessed)
			}

			t.Logf("Lowest unprocessed : %d", readChannel.lowestUnprocessed)

			if readChannel.lowestUnprocessed != uint32(test.processCount) {
				t.Errorf("Wrong lowest unprocessed : got %d, want %d",
					readChannel.lowestUnprocessed, test.processCount)
			}

			firstLoaded := uint64(readChannel.lowestUnprocessed)
			firstLoaded -= firstLoaded % messagesPerFile
			if firstLoaded >= uint64(messagesPerFile) &&
				firstLoaded+uint64(messagesPerFile) > uint64(test.total) {
				firstLoaded = uint64(test.total - messagesPerFile)
				firstLoaded -= firstLoaded % messagesPerFile
			}

			if firstLoaded != currentID {
				t.Errorf("Wrong first loaded : got %d, want %d", currentID, firstLoaded)
			}

			verifiedCount := 0
			for _, message := range readChannel.messages {
				if message.ID() != currentID {
					t.Errorf("Wrong id : got %d, want %d", message.ID(), currentID)
				}
				script := messages[currentID]

				if !message.Payload().Equal(script) {
					t.Errorf("Wrong message at %d", currentID)
				} else {
					verifiedCount++
				}
				currentID++
			}

			t.Logf("Verified %d messages", verifiedCount)
		})
	}
}
