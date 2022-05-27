package wallet

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

type MockMerkleProofVerifier struct {
	blocks map[bitcoin.Hash32]*mockMerkleBlock

	currentTime       time.Time
	currentHeight     int
	previousBlockHash *bitcoin.Hash32

	lock sync.Mutex
}

type mockMerkleBlock struct {
	header *wire.BlockHeader
	height int
}

func NewMockMerkleProofVerifier() *MockMerkleProofVerifier {
	return &MockMerkleProofVerifier{
		blocks:        make(map[bitcoin.Hash32]*mockMerkleBlock),
		currentTime:   time.Now().Add(-10 * time.Hour),
		currentHeight: 1000,
	}
}

// MockMerkleProofs creates a mock header that contains a merkle root that corresponds to a valid
// merkle proof for a specific txid. The header is retained so a later call the VerifyMerkleProof
// will be able to validate it.
func (m *MockMerkleProofVerifier) MockMerkleProofs(txids ...bitcoin.Hash32) []*merkle_proof.MerkleProof {
	header := &wire.BlockHeader{
		Timestamp: uint32(m.currentTime.Unix()),
		Bits:      0x1d00ffff,
		Nonce:     rand.Uint32(),
	}

	if m.previousBlockHash == nil {
		rand.Read(header.PrevBlock[:])
	} else {
		copy(header.PrevBlock[:], (*m.previousBlockHash)[:])
	}
	m.currentTime = m.currentTime.Add(time.Minute * 10)

	tree := merkle_proof.NewMerkleTree(true)

	txCount := rand.Intn(100)
	var offsets []int
	for _, txid := range txids {
		offsets = append(offsets, txCount)
		tree.AddMerkleProof(txid)
		txCount += 1 + rand.Intn(100)
	}

	offsetIndex := 0
	for i := 0; i < txCount; i++ {
		if offsetIndex < len(offsets) && i == offsets[offsetIndex] {
			tree.AddHash(txids[offsetIndex])
			offsetIndex++
			continue
		}

		var otherTxid bitcoin.Hash32
		rand.Read(otherTxid[:])
		tree.AddHash(otherTxid)
	}

	if offsetIndex != len(offsets) {
		panic("all offsets not hit")
	}

	merkleRoot, proofs := tree.FinalizeMerkleProofs()
	copy(header.MerkleRoot[:], merkleRoot[:])

	blockHash := *header.BlockHash()
	m.previousBlockHash = &blockHash

	for i, proof := range proofs {
		proof.TxID = &txids[i]
		proof.BlockHash = &bitcoin.Hash32{}
		copy(proof.BlockHash[:], blockHash[:])
	}

	m.lock.Lock()
	m.blocks[blockHash] = &mockMerkleBlock{
		header: header,
		height: m.currentHeight,
	}
	m.currentHeight++
	m.lock.Unlock()

	return proofs
}

func (m *MockMerkleProofVerifier) VerifyMerkleProof(ctx context.Context,
	proof *merkle_proof.MerkleProof) (int, bool, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var height int
	if proof.BlockHeader != nil {
		hash := *proof.BlockHeader.BlockHash()

		block, exists := m.blocks[hash]
		if !exists {
			return -1, false, errors.Wrap(ErrUnknownHeader, hash.String())
		}

		height = block.height

	} else if proof.BlockHash != nil {
		block, exists := m.blocks[*proof.BlockHash]
		if !exists {
			return -1, false, errors.Wrap(ErrUnknownHeader, proof.BlockHash.String())
		}

		proof.BlockHeader = block.header
		height = block.height

	} else {
		return -1, false, merkle_proof.ErrNotVerifiable
	}

	if err := proof.Verify(); err != nil {
		return -1, false, errors.Wrap(err, "merkle proof")
	}

	return height, true, nil
}
