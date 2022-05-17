package wallet

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"time"

	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

type Key struct {
	Hash          bitcoin.Hash32 `json:"hash"`
	LockingScript bitcoin.Script `json:"locking_script"`
	Key           bitcoin.Key    `json:"key"`
}

type Keys []*Key

type KeySet map[bitcoin.Hash32]Keys

func (w *Wallet) GetKeys(contextID bitcoin.Hash32) (Keys, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	keys, exists := w.KeySet[contextID]
	if !exists {
		return nil, ErrContextNotFound
	}

	return keys, nil
}

// GenerateKey generates a new hash and derives a new key from the base key and the hash.
func (w *Wallet) GenerateKey(contextID bitcoin.Hash32) (*Key, error) {
	w.lock.RLock()
	baseKey := w.baseKey
	w.lock.RUnlock()

	hash, key := GenerateHashKey(baseKey, contextID)

	lockingScript, err := key.LockingScript()
	if err != nil {
		return nil, errors.Wrap(err, "locking script")
	}

	walletKey := &Key{
		Hash:          hash,
		LockingScript: lockingScript,
		Key:           key,
	}

	w.lock.Lock()
	previous, exists := w.KeySet[contextID]
	if exists {
		w.KeySet[contextID] = append(previous, walletKey)
	} else {
		w.KeySet[contextID] = Keys{walletKey}
	}
	w.lock.Unlock()

	return walletKey, nil
}

// GenerateKey generates a new hash and derives a new key from the base key and the hash.
func (w *Wallet) GenerateKeys(contextID bitcoin.Hash32, count int) (Keys, error) {
	w.lock.RLock()
	baseKey := w.baseKey
	w.lock.RUnlock()

	hash := GenerateHash(contextID)

	result := make(Keys, count)
	for i := range result {
		for {
			key, err := baseKey.AddHash(hash)
			if err != nil {
				if errors.Cause(err) == bitcoin.ErrOutOfRangeKey {
					hash = IncrementHash(hash)
					continue
				}
				return nil, errors.Wrap(err, "key")
			}

			lockingScript, err := key.LockingScript()
			if err != nil {
				return nil, errors.Wrap(err, "locking script")
			}

			walletKey := &Key{
				LockingScript: lockingScript,
				Key:           key,
			}
			copy(walletKey.Hash[:], (hash)[:])
			result[i] = walletKey

			hash = IncrementHash(hash)
			break
		}
	}

	w.lock.Lock()
	previous, exists := w.KeySet[contextID]
	if exists {
		w.KeySet[contextID] = append(previous, result...)
	} else {
		w.KeySet[contextID] = result
	}
	w.lock.Unlock()

	return result, nil
}

func GenerateHashKey(baseKey bitcoin.Key, contextID bitcoin.Hash32) (bitcoin.Hash32, bitcoin.Key) {
	for {
		hash := GenerateHash(contextID)
		key, err := baseKey.AddHash(hash)
		if err != nil {
			continue // should only be out of range key
		}

		return hash, key
	}
}

func IncrementHash(hash bitcoin.Hash32) bitcoin.Hash32 {
	hasher := sha256.New()
	hasher.Write(hash[:])

	randomBytes := make([]byte, bitcoin.Hash32Size)
	rand.Read(randomBytes)
	hasher.Write(randomBytes)

	newHash := sha256.Sum256(hasher.Sum(nil))
	result, _ := bitcoin.NewHash32(newHash[:])
	return *result
}

// GenerateHash creates a random hash value that is used to derive a new key.
func GenerateHash(contextID bitcoin.Hash32) bitcoin.Hash32 {
	hasher := sha256.New()
	hasher.Write(contextID[:])

	randomBytes := make([]byte, bitcoin.Hash32Size)
	rand.Read(randomBytes)
	hasher.Write(randomBytes)

	timeBytes, _ := time.Now().MarshalBinary()
	hasher.Write(timeBytes)

	hash := sha256.Sum256(hasher.Sum(nil))
	result, _ := bitcoin.NewHash32(hash[:])
	return *result
}

func RandomHash() bitcoin.Hash32 {
	hasher := sha256.New()

	randomBytes := make([]byte, bitcoin.Hash32Size)
	rand.Read(randomBytes)
	hasher.Write(randomBytes)

	timeBytes, _ := time.Now().MarshalBinary()
	hasher.Write(timeBytes)

	hash := sha256.Sum256(hasher.Sum(nil))
	result, _ := bitcoin.NewHash32(hash[:])
	return *result
}

func (k *KeySet) Serialize(w io.Writer) error {
	if err := binary.Write(w, endian, uint32(len(*k))); err != nil {
		return errors.Wrap(err, "map count")
	}

	for contextID, keys := range *k {
		if err := contextID.Serialize(w); err != nil {
			return errors.Wrapf(err, "context id %s", contextID)
		}

		if err := binary.Write(w, endian, uint32(len(keys))); err != nil {
			return errors.Wrapf(err, "key count %s", contextID)
		}

		for i, key := range keys {
			if err := key.Hash.Serialize(w); err != nil {
				return errors.Wrapf(err, "key hash %d %s", i, contextID)
			}
		}
	}

	return nil
}

func (k *KeySet) Deserialize(r io.Reader) error {
	result := make(map[bitcoin.Hash32]Keys)

	var mapCount uint32
	if err := binary.Read(r, endian, &mapCount); err != nil {
		return errors.Wrap(err, "map count")
	}

	for m := uint32(0); m < mapCount; m++ {
		var contextID bitcoin.Hash32
		if err := contextID.Deserialize(r); err != nil {
			return errors.Wrapf(err, "context id %d", m)
		}

		var keyCount uint32
		if err := binary.Read(r, endian, &keyCount); err != nil {
			return errors.Wrapf(err, "key count %d", m)
		}

		keys := make(Keys, keyCount)
		for i := range keys {
			newKey := &Key{}
			if err := newKey.Hash.Deserialize(r); err != nil {
				return errors.Wrapf(err, "key hash %d %d", i, m)
			}

			keys[i] = newKey
		}

		result[contextID] = keys
	}

	*k = result
	return nil
}
