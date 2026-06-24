package oracle

import (
	"context"
	"encoding/json"

	"github.com/fiatjaf/eventstore"
	"github.com/nbd-wtf/go-nostr"
)

func (b *OracleBackend) SaveEvent(ctx context.Context, evt *nostr.Event) error {
	tagsj, _ := json.Marshal(evt.Tags)

	res, err := b.DB.ExecContext(ctx, `
		MERGE INTO event dst
		USING (SELECT :1 AS id FROM dual) src
		ON (dst.id = src.id)
		WHEN NOT MATCHED THEN
			INSERT (id, pubkey, created_at, kind, tags, content, sig)
			VALUES (:1, :2, :3, :4, :5, :6, :7)
	`, evt.ID, evt.PubKey, evt.CreatedAt, evt.Kind, string(tagsj), evt.Content, evt.Sig)
	if err != nil {
		return err
	}

	nr, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if nr == 0 {
		return eventstore.ErrDupEvent
	}

	return nil
}
