package wallet

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

const (
	keysPath        = "channels_wallet/keys"
	archiveKeysPath = "channels_wallet/archived_keys"
	keysVersion     = uint8(0)
)

type Key struct {
	Hash          bitcoin.Hash32 `json:"hash"`
	LockingScript bitcoin.Script `json:"locking_script"`
	Key           bitcoin.Key    `json:"key"`
	UsedHeight    uint32         `json:"used_height"`

	modified bool // when true this key needs to be written to storage
}

type Keys []*Key

type KeySet map[bitcoin.Hash32]Keys

func (w *Wallet) GetKeys(ctx context.Context, contextID bitcoin.Hash32) (Keys, error) {
	w.keysLock.Lock()
	defer w.keysLock.Unlock()

	keys, exists := w.keys[contextID]
	if !exists {
		return nil, errors.Wrap(ErrContextIDNotFound, contextID.String())
	}

	newKeys := make(Keys, len(keys))
	copy(newKeys, keys)
	return newKeys, nil
}

func (w *Wallet) GetKeyForHash(hash bitcoin.Hash32) *Key {
	w.keysLock.RLock()
	defer w.keysLock.RUnlock()

	for _, keys := range w.keys {
		for _, key := range keys {
			if key.Hash.Equal(&hash) {
				return key
			}
		}
	}

	return nil
}

func (w *Wallet) GetKeyForLockingScript(script bitcoin.Script) *Key {
	w.keysLock.RLock()
	defer w.keysLock.RUnlock()

	for _, keys := range w.keys {
		for _, key := range keys {
			if key.LockingScript.Equal(script) {
				return key
			}
		}
	}

	return nil
}

func (w *Wallet) FindKeys(ctx context.Context, tx *wire.MsgTx) (Keys, *bitcoin.Hash32, error) {
	w.keysLock.Lock()
	defer w.keysLock.Unlock()

	for _, txout := range tx.TxOut {
		if txout.Value == 0 {
			continue
		}

		for contextID, keys := range w.keys {
			for _, key := range keys {
				if key.LockingScript.Equal(txout.LockingScript) {
					newKeys := make(Keys, len(keys))
					copy(newKeys, keys)
					return newKeys, &contextID, nil
				}
			}
		}
	}

	return nil, nil, ErrContextIDNotFound
}

// GenerateKey generates a new hash and derives a new key from the base key and the hash. This key
// is not saved until RetainKeys is called.
func (w *Wallet) GenerateKey(contextID bitcoin.Hash32) (*Key, error) {
	baseKey := w.BaseKey()

	hash, key := GenerateHashKey(baseKey, contextID)

	lockingScript, err := key.LockingScript()
	if err != nil {
		return nil, errors.Wrap(err, "locking script")
	}

	walletKey := &Key{
		Hash:          hash,
		LockingScript: lockingScript,
		Key:           key,
		modified:      true,
	}

	return walletKey, nil
}

// GenerateKey generates a new hash and derives a new key from the base key and the hash. These keys
// are not saved until RetainKeys is called.
func (w *Wallet) GenerateKeys(contextID bitcoin.Hash32, count int) (Keys, error) {
	baseKey := w.BaseKey()

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
				modified:      true,
			}
			copy(walletKey.Hash[:], (hash)[:])
			result[i] = walletKey

			hash = IncrementHash(hash)
			break
		}
	}

	return result, nil
}

func (w *Wallet) RetainKeys(contextID bitcoin.Hash32, keys Keys) error {
	w.keysLock.Lock()
	defer w.keysLock.Unlock()

	previous, exists := w.keys[contextID]
	if exists {
		w.keys[contextID] = append(previous, keys...)
	} else {
		w.keys[contextID] = keys
	}

	return nil
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

func RandomHashPtr() *bitcoin.Hash32 {
	hasher := sha256.New()

	randomBytes := make([]byte, bitcoin.Hash32Size)
	rand.Read(randomBytes)
	hasher.Write(randomBytes)

	timeBytes, _ := time.Now().MarshalBinary()
	hasher.Write(timeBytes)

	hash := sha256.Sum256(hasher.Sum(nil))
	result, _ := bitcoin.NewHash32(hash[:])
	return result
}

func (ks *KeySet) load(ctx context.Context, store storage.StreamStorage,
	baseKey bitcoin.Key) error {

	paths, err := store.List(ctx, keysPath)
	if err != nil {
		return errors.Wrap(err, "list")
	}

	for _, path := range paths {
		var keySet KeySet
		if err := storage.StreamRead(ctx, store, path, &keySet); err != nil {
			return errors.Wrapf(err, "read %s", path)
		}

		for contextID, keys := range keySet {
			(*ks)[contextID] = keys
		}
	}

	// Recalculate keys and locking scripts from hashes.
	for _, keys := range *ks {
		for _, walletKey := range keys {
			key, err := baseKey.AddHash(walletKey.Hash)
			if err != nil {
				return errors.Wrap(err, "key")
			}
			walletKey.Key = key

			lockingScript, err := key.LockingScript()
			if err != nil {
				return errors.Wrap(err, "locking script")
			}
			walletKey.LockingScript = lockingScript
		}
	}

	return nil
}

func (ks *KeySet) save(ctx context.Context, store storage.StreamStorage, blockHeight uint32) error {
	paths, err := store.List(ctx, keysPath)
	if err != nil {
		return errors.Wrap(err, "list")
	}

	parts := ks.split()
	pruneHeight := uint32(0)
	if blockHeight > pruneDepth {
		pruneHeight = blockHeight - pruneDepth
	}
	newKeySet := make(KeySet)

	for lookup, keyset := range parts {
		// Archive any keys that have been used already.
		modified := false
		archiveKeys := make(KeySet)
		unarchivedKeys := make(KeySet)
		for contextID, keys := range keyset {
			for _, key := range keys {
				if key.modified {
					modified = true
				}

				if key.UsedHeight > 0 && key.UsedHeight < pruneHeight {
					k, exists := archiveKeys[contextID]
					if exists {
						archiveKeys[contextID] = append(k, key)
					} else {
						archiveKeys[contextID] = Keys{key}
					}
					modified = true
				} else {
					k, exists := unarchivedKeys[contextID]
					if exists {
						unarchivedKeys[contextID] = append(k, key)
					} else {
						unarchivedKeys[contextID] = Keys{key}
					}
				}
			}
		}

		path := fmt.Sprintf("%s/0x%02x", keysPath, lookup)
		if !modified {
			if len(unarchivedKeys) > 0 {
				for contextID, keys := range unarchivedKeys {
					for _, key := range keys {
						key.modified = false
					}

					newKeySet[contextID] = keys
				}

				paths = removePath(paths, path)
			}
			continue
		}

		if len(unarchivedKeys) > 0 {
			if err := storage.StreamWrite(ctx, store, path, &unarchivedKeys); err != nil {
				return errors.Wrapf(err, "write %s", path)
			}

			for contextID, keys := range unarchivedKeys {
				for _, key := range keys {
					key.modified = false
				}

				newKeySet[contextID] = keys
			}

			paths = removePath(paths, path)
		}

		if len(archiveKeys) > 0 {
			archivePath := fmt.Sprintf("%s/0x%02x", archiveKeysPath, lookup)

			// Read previously archived keys.
			archivedKeys := make(KeySet)
			if err := storage.StreamRead(ctx, store, archivePath, &archivedKeys); err != nil {
				if errors.Cause(err) == storage.ErrNotFound {
					archivedKeys = make(KeySet) // make sure it is empty after error
				} else {
					return errors.Wrapf(err, "read %s", archivePath)
				}
			}

			// Append new archive keys.
			for contextID, keys := range archiveKeys {
				k, exists := archivedKeys[contextID]
				if exists {
					archivedKeys[contextID] = append(k, keys...)
				} else {
					archivedKeys[contextID] = keys
				}
			}

			if err := storage.StreamWrite(ctx, store, archivePath, &archivedKeys); err != nil {
				return errors.Wrapf(err, "write %s", archivePath)
			}
		}
	}

	// Remove any parts that no longer exist.
	for _, path := range paths {
		if err := store.Remove(ctx, path); err != nil {
			return errors.Wrapf(err, "remove %s", path)
		}
	}

	*ks = newKeySet
	return nil
}

func (ks KeySet) split() map[byte]KeySet {
	result := make(map[byte]KeySet)
	for contextID, keys := range ks {
		lookup := contextID[0]
		set, exists := result[lookup]
		if exists {
			set[contextID] = keys
			result[lookup] = set
		} else {
			set = make(KeySet)
			set[contextID] = keys
			result[lookup] = set
		}
	}

	return result
}

func (ks *KeySet) Serialize(w io.Writer) error {
	if err := binary.Write(w, endian, keysVersion); err != nil {
		return errors.Wrap(err, "version")
	}

	if err := binary.Write(w, endian, uint32(len(*ks))); err != nil {
		return errors.Wrap(err, "map count")
	}

	for contextID, keys := range *ks {
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

			if err := binary.Write(w, endian, key.UsedHeight); err != nil {
				return errors.Wrapf(err, "key height %d %s", i, contextID)
			}
		}
	}

	return nil
}

func (ks *KeySet) Deserialize(r io.Reader) error {
	var version uint8
	if err := binary.Read(r, endian, &version); err != nil {
		return errors.Wrap(err, "version")
	}
	if version != 0 {
		return errors.New("Unsupported version")
	}

	var mapCount uint32
	if err := binary.Read(r, endian, &mapCount); err != nil {
		return errors.Wrap(err, "map count")
	}

	result := make(map[bitcoin.Hash32]Keys)
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

			if err := binary.Read(r, endian, &newKey.UsedHeight); err != nil {
				return errors.Wrapf(err, "key height %d %s", i, contextID)
			}

			keys[i] = newKey
		}

		result[contextID] = keys
	}

	*ks = result
	return nil
}
