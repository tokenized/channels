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

	for {
		hash, err := GenerateHash(s)
		if err != nil {
			return nil, errors.Wrap(err, "hash")
		}

		key, err := baseKey.AddHash(*hash)
		if err != nil {
			if errors.Cause(err) == bitcoin.ErrOutOfRangeKey {
				continue
			}
			return nil, errors.Wrap(err, "key")
		}

		lockingScript, err := key.LockingScript()
		if err != nil {
			return nil, errors.Wrap(err, "locking script")
		}

		walletKey := &Key{
			Hash:          *hash,
			LockingScript: lockingScript,
			Key:           key,
		}

		w.lock.Lock()
		w.KeySet = append(w.KeySet, Keys{walletKey})
		w.lock.Unlock()

		return walletKey, nil
	}
}

// GenerateKey generates a new hash and derives a new key from the base key and the hash.
func (w *Wallet) GenerateKeys(s string, count int) (Keys, error) {
	w.lock.RLock()
	baseKey := w.baseKey
	w.lock.RUnlock()

	hash, err := GenerateHash(s)
	if err != nil {
		return nil, errors.Wrap(err, "hash")
	}

	result := make(Keys, count)
	for i := range result {
		for {
			key, err := baseKey.AddHash(*hash)
			if err != nil {
				if errors.Cause(err) == bitcoin.ErrOutOfRangeKey {
					hash, err = IncrementHash(*hash)
					if err != nil {
						return nil, errors.Wrap(err, "increment")
					}
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
			copy(walletKey.Hash[:], (*hash)[:])
			result[i] = walletKey

			hash, err = IncrementHash(*hash)
			if err != nil {
				return nil, errors.Wrap(err, "increment")
			}
			break
		}
	}

	w.lock.Lock()
	w.KeySet = append(w.KeySet, result)
	w.lock.Unlock()

	return result, nil
}

func IncrementHash(hash bitcoin.Hash32) (*bitcoin.Hash32, error) {
	hasher := sha256.New()
	hasher.Write(hash[:])

	randomBytes := make([]byte, bitcoin.Hash32Size)
	rand.Read(randomBytes)
	hasher.Write(randomBytes)

	newHash := sha256.Sum256(hasher.Sum(nil))
	result, err := bitcoin.NewHash32(newHash[:])
	if err != nil {
		return nil, errors.Wrap(err, "hash32")
	}

	return result, nil
}

// GenerateHash creates a random hash value that is used to derive a new key.
func GenerateHash(s string) (*bitcoin.Hash32, error) {
	hasher := sha256.New()
	hasher.Write([]byte(s))

	randomBytes := make([]byte, bitcoin.Hash32Size)
	rand.Read(randomBytes)
	hasher.Write(randomBytes)

	timeBytes, err := time.Now().MarshalBinary()
	if err != nil {
		return nil, errors.Wrap(err, "time")
	}
	hasher.Write(timeBytes)

	hash := sha256.Sum256(hasher.Sum(nil))
	result, err := bitcoin.NewHash32(hash[:])
	if err != nil {
		return nil, errors.Wrap(err, "hash32")
	}

	return result, nil
}
