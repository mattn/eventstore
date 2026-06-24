package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

func (b *OracleBackend) QueryEvents(ctx context.Context, filter nostr.Filter) (ch chan *nostr.Event, err error) {
	query, params, err := b.queryEventsSql(filter, false)
	if err != nil {
		return nil, err
	}

	rows, err := b.DB.QueryContext(ctx, query, params...)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to fetch events using query %q: %w", query, err)
	}

	ch = make(chan *nostr.Event)
	go func() {
		defer rows.Close()
		defer close(ch)
		for rows.Next() {
			var evt nostr.Event
			var timestamp int64
			var tagsJSON string
			err := rows.Scan(&evt.ID, &evt.PubKey, &timestamp,
				&evt.Kind, &tagsJSON, &evt.Content, &evt.Sig)
			if err != nil {
				return
			}
			evt.CreatedAt = nostr.Timestamp(timestamp)
			if err := json.Unmarshal([]byte(tagsJSON), &evt.Tags); err != nil {
				return
			}
			select {
			case ch <- &evt:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

func (b *OracleBackend) CountEvents(ctx context.Context, filter nostr.Filter) (int64, error) {
	query, params, err := b.queryEventsSql(filter, true)
	if err != nil {
		return 0, err
	}

	var count int64
	if err = b.DB.QueryRowContext(ctx, query, params...).Scan(&count); err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to fetch events using query %q: %w", query, err)
	}
	return count, nil
}

func makePlaceHolders(start, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = fmt.Sprintf(":%d", start+i)
	}
	return strings.Join(parts, ",")
}

var (
	TooManyIDs       = errors.New("too many ids")
	TooManyAuthors   = errors.New("too many authors")
	TooManyKinds     = errors.New("too many kinds")
	TooManyTagValues = errors.New("too many tag values")
	EmptyTagSet      = errors.New("empty tag set")
)

func (b *OracleBackend) queryEventsSql(filter nostr.Filter, doCount bool) (string, []any, error) {
	conditions := make([]string, 0, 7)
	params := make([]any, 0, 20)
	paramIdx := 1

	if len(filter.IDs) > 0 {
		if len(filter.IDs) > b.QueryIDsLimit {
			return "", nil, TooManyIDs
		}

		for _, v := range filter.IDs {
			params = append(params, v)
		}
		conditions = append(conditions, `id IN (`+makePlaceHolders(paramIdx, len(filter.IDs))+`)`)
		paramIdx += len(filter.IDs)
	}

	if len(filter.Authors) > 0 {
		if len(filter.Authors) > b.QueryAuthorsLimit {
			return "", nil, TooManyAuthors
		}

		for _, v := range filter.Authors {
			params = append(params, v)
		}
		conditions = append(conditions, `pubkey IN (`+makePlaceHolders(paramIdx, len(filter.Authors))+`)`)
		paramIdx += len(filter.Authors)
	}

	if len(filter.Kinds) > 0 {
		if len(filter.Kinds) > b.QueryKindsLimit {
			return "", nil, TooManyKinds
		}

		for _, v := range filter.Kinds {
			params = append(params, v)
		}
		conditions = append(conditions, `kind IN (`+makePlaceHolders(paramIdx, len(filter.Kinds))+`)`)
		paramIdx += len(filter.Kinds)
	}

	totalTags := 0
	for _, values := range filter.Tags {
		if len(values) == 0 {
			return "", nil, EmptyTagSet
		}

		totalTags += len(values)
		if totalTags > b.QueryTagsLimit {
			return "", nil, TooManyTagValues
		}

		tagConditions := make([]string, len(values))
		for i, tagValue := range values {
			params = append(params, tagValue)
			tagConditions[i] = fmt.Sprintf("JSON_EXISTS(tags, '$[*][1]?(@ == :%d)')", paramIdx)
			paramIdx++
		}
		conditions = append(conditions, "("+strings.Join(tagConditions, " OR ")+")")
	}

	if filter.Since != nil {
		conditions = append(conditions, fmt.Sprintf(`created_at >= :%d`, paramIdx))
		params = append(params, *filter.Since)
		paramIdx++
	}
	if filter.Until != nil {
		conditions = append(conditions, fmt.Sprintf(`created_at <= :%d`, paramIdx))
		params = append(params, *filter.Until)
		paramIdx++
	}

	if len(conditions) == 0 {
		conditions = append(conditions, `1=1`)
	}

	limit := b.QueryLimit
	if filter.Limit > 0 && filter.Limit <= b.QueryLimit {
		limit = filter.Limit
	}
	params = append(params, limit)

	var query string
	if doCount {
		query = fmt.Sprintf(`SELECT COUNT(*) FROM event WHERE %s AND ROWNUM <= :%d`,
			strings.Join(conditions, " AND "), paramIdx)
	} else {
		query = fmt.Sprintf(`SELECT id, pubkey, created_at, kind, tags, content, sig
			FROM event WHERE %s ORDER BY created_at DESC, id FETCH FIRST :%d ROWS ONLY`,
			strings.Join(conditions, " AND "), paramIdx)
	}

	return query, params, nil
}
