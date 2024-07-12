package eventstore

import (
	"strconv"

	"github.com/nbd-wtf/go-nostr"
)

func isOlder(previous, next *nostr.Event) bool {
	return previous.CreatedAt < next.CreatedAt ||
		(previous.CreatedAt == next.CreatedAt && previous.ID > next.ID)
}

func Expired(ev *nostr.Event) bool {
	exp := ev.Tags.GetAll([]string{"expiration"})
	if len(exp) == 0 {
		return false
	}

	now := nostr.Now()
	for _, ex := range exp {
		i, err := strconv.ParseUint(ex.Value(), 10, 64)
		if err != nil || nostr.Timestamp(i) <= now {
			return true
		}
	}
	return false
}
