package badger

import (
	"context"
	"encoding/hex"

	"github.com/dgraph-io/badger/v4"
	"github.com/nbd-wtf/go-nostr"
	nostr_binary "github.com/nbd-wtf/go-nostr/binary"
)

func (b *BadgerBackend) DeleteEvent(ctx context.Context, evt *nostr.Event) error {
	deletionHappened := false

	err := b.Update(func(txn *badger.Txn) error {
		idx := make([]byte, 1, 5)
		idx[0] = rawEventStorePrefix

		// query event by id to get its idx
		id, _ := hex.DecodeString(evt.ID)
		prefix := make([]byte, 1+32)
		prefix[0] = indexIdPrefix
		copy(prefix[1:], id)
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		it.Seek(prefix)
		if it.ValidForPrefix(prefix) {
			idx = append(idx, it.Item().Key()[1+32:]...)
		}
		it.Close()

		// if no idx was found, end here, this event doesn't exist
		if len(idx) == 1 {
			return nil
		}

		// fetch the event
		item, err := txn.Get(idx)
		if err != nil {
			return err
		}

		item.Value(func(val []byte) error {
			evt := &nostr.Event{}
			if err := nostr_binary.Unmarshal(val, evt); err != nil {
				return err
			}

			// set this so we'll run the GC later
			deletionHappened = true

			// calculate all index keys we have for this event and delete them
			for _, k := range getIndexKeysForEvent(evt, idx[1:]) {
				if err := txn.Delete(k); err != nil {
					return err
				}
			}

			// delete the raw event
			return txn.Delete(idx)
		})

		return nil
	})
	if err != nil {
		return err
	}

	// after deleting, run garbage collector
	if deletionHappened {
		if err := b.RunValueLogGC(0.8); err != nil {
			panic(err)
		}
	}

	return nil
}
