package wallet

import (
	"crypto/rand"
	"crypto/sha256"
	"time"

	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

type Key struct {
	Hash          bitcoin.Hash32 `bsor:"1" json:"hash"`
	LockingScript bitcoin.Script `bsor:"-" json:"-"`
	Key           bitcoin.Key    `bsor:"-" json:"-"`
}

type Keys []*Key

type KeySet []Keys

// GenerateKey generates a new hash and derives a new key from the base key and the hash.
func (w *Wallet) GenerateKey(s string) (*Key, error) {
	w.lock.RLock()
	baseKey := w.baseKey
	w.lock.RUnlock()

	hash, key := GenerateHashKey(baseKey, s)

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
	w.KeySet = append(w.KeySet, Keys{walletKey})
	w.lock.Unlock()

	return walletKey, nil
}

// GenerateKey generates a new hash and derives a new key from the base key and the hash.
func (w *Wallet) GenerateKeys(s string, count int) (Keys, error) {
	w.lock.RLock()
	baseKey := w.baseKey
	w.lock.RUnlock()

	hash := GenerateHash(s)

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
	w.KeySet = append(w.KeySet, result)
	w.lock.Unlock()

	return result, nil
}

func GenerateHashKey(baseKey bitcoin.Key, s string) (bitcoin.Hash32, bitcoin.Key) {
	for {
		hash := GenerateHash(s)
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
func GenerateHash(s string) bitcoin.Hash32 {
	hasher := sha256.New()
	hasher.Write([]byte(s))

	randomBytes := make([]byte, bitcoin.Hash32Size)
	rand.Read(randomBytes)
	hasher.Write(randomBytes)

	timeBytes, _ := time.Now().MarshalBinary()
	hasher.Write(timeBytes)

	hash := sha256.Sum256(hasher.Sum(nil))
	result, _ := bitcoin.NewHash32(hash[:])
	return *result
}
