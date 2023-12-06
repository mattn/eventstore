package lmdb

import (
	"encoding/binary"
	"encoding/hex"
	"strings"

	"github.com/PowerDNS/lmdb-go/lmdb"
	"github.com/fiatjaf/eventstore"
	"github.com/nbd-wtf/go-nostr"
	"golang.org/x/exp/slices"
)

func (b *LMDBBackend) getTagIndexPrefix(tagValue string) (lmdb.DBI, []byte, int) {
	var k []byte   // the key with full length for created_at and idx at the end, but not filled with these
	var offset int // the offset -- i.e. where the prefix ends and the created_at and idx would start
	var dbi lmdb.DBI

	if kind, pkb, d := eventstore.GetAddrTagElements(tagValue); len(pkb) == 32 {
		// store value in the new special "a" tag index
		k = make([]byte, 2+8+len(d)+4)
		binary.BigEndian.PutUint16(k[1:], kind)
		copy(k[2:], pkb[0:8])
		copy(k[2+8:], d)
		offset = 2 + 8 + len(d)
		dbi = b.indexTagAddr
	} else if vb, _ := hex.DecodeString(tagValue); len(vb) == 32 {
		// store value as bytes
		k = make([]byte, 8+4)
		copy(k, vb[0:8])
		offset = 8
		dbi = b.indexTag32
	} else {
		// store whatever as utf-8
		k = make([]byte, len(tagValue)+4)
		copy(k, tagValue)
		offset = len(tagValue)
		dbi = b.indexTag
	}

	return dbi, k, offset
}

func (b *LMDBBackend) getIndexKeysForEvent(evt *nostr.Event) []key {
	keys := make([]key, 0, 18)

	// indexes
	{
		// ~ by id
		k, _ := hex.DecodeString(evt.ID)
		keys = append(keys, key{dbi: b.indexId, key: k})
	}

	{
		// ~ by pubkey+date
		pubkey, _ := hex.DecodeString(evt.PubKey)
		k := make([]byte, 8+4)
		copy(k[:], pubkey[0:8])
		binary.BigEndian.PutUint32(k[8:], uint32(evt.CreatedAt))
		keys = append(keys, key{dbi: b.indexPubkey, key: k})
	}

	{
		// ~ by kind+date
		k := make([]byte, 2+4)
		binary.BigEndian.PutUint16(k[:], uint16(evt.Kind))
		binary.BigEndian.PutUint32(k[2:], uint32(evt.CreatedAt))
		keys = append(keys, key{dbi: b.indexKind, key: k})
	}

	{
		// ~ by pubkey+kind+date
		pubkey, _ := hex.DecodeString(evt.PubKey)
		k := make([]byte, 8+2+4)
		copy(k[:], pubkey[0:8])
		binary.BigEndian.PutUint16(k[8:], uint16(evt.Kind))
		binary.BigEndian.PutUint32(k[8+2:], uint32(evt.CreatedAt))
		keys = append(keys, key{dbi: b.indexPubkeyKind, key: k})
	}

	// ~ by tagvalue+date
	slices.SortFunc(evt.Tags, func(a, b nostr.Tag) int { return strings.Compare(a[1], b[1]) })
	for i, tag := range evt.Tags {
		if len(tag) < 2 || len(tag[0]) != 1 || len(tag[1]) == 0 || len(tag[1]) > 100 {
			// not indexable
			continue
		}
		if i > 0 && evt.Tags[i-1][1] == tag[1] {
			// duplicate
			continue
		}

		// get key prefix (with full length) and offset where to write the created_at
		dbi, k, offset := b.getTagIndexPrefix(tag[1])

		// write the created_at
		binary.BigEndian.PutUint32(k[offset:], uint32(evt.CreatedAt))

		keys = append(keys, key{dbi: dbi, key: k})
	}

	{
		// ~ by date only
		k := make([]byte, 4)
		binary.BigEndian.PutUint32(k[:], uint32(evt.CreatedAt))
		keys = append(keys, key{dbi: b.indexCreatedAt, key: k})
	}

	return keys
}
