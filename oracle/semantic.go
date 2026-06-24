package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/generativeaiinference"
)

type SemanticSearchResult struct {
	Event      *nostr.Event
	Similarity float64
}

func (b *OracleBackend) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	provider, err := b.getConfigProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to get OCI config: %w", err)
	}

	client, err := generativeaiinference.NewGenerativeAiInferenceClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create generative AI client: %w", err)
	}

	truncate := generativeaiinference.EmbedTextDetailsTruncateEnd
	req := generativeaiinference.EmbedTextRequest{
		EmbedTextDetails: generativeaiinference.EmbedTextDetails{
			CompartmentId: common.String(b.OCICompartmentID),
			ServingMode: generativeaiinference.OnDemandServingMode{
				ModelId: common.String(b.EmbeddingModel),
			},
			Inputs:   []string{text},
			Truncate: truncate,
		},
	}

	resp, err := client.EmbedText(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	if len(resp.EmbedTextResult.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return resp.EmbedTextResult.Embeddings[0], nil
}

func (b *OracleBackend) getConfigProvider() (common.ConfigurationProvider, error) {
	if b.OCIConfigPath != "" {
		return common.ConfigurationProviderFromFileWithProfile(b.OCIConfigPath, b.OCIProfile, "")
	}
	return common.DefaultConfigProvider(), nil
}

func (b *OracleBackend) UpdateEventEmbedding(ctx context.Context, eventID string, embedding []float32) error {
	vectorStr := formatVector(embedding)
	_, err := b.DB.ExecContext(ctx, `
		UPDATE event SET embedding = :1 WHERE id = :2
	`, vectorStr, eventID)
	return err
}

func (b *OracleBackend) SaveEventWithEmbedding(ctx context.Context, evt *nostr.Event) error {
	embedding, err := b.GenerateEmbedding(ctx, evt.Content)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	tagsj, _ := json.Marshal(evt.Tags)
	vectorStr := formatVector(embedding)

	_, err = b.DB.ExecContext(ctx, `
		MERGE INTO event dst
		USING (SELECT :1 AS id FROM dual) src
		ON (dst.id = src.id)
		WHEN NOT MATCHED THEN
			INSERT (id, pubkey, created_at, kind, tags, content, sig, embedding)
			VALUES (:1, :2, :3, :4, :5, :6, :7, :8)
	`, evt.ID, evt.PubKey, evt.CreatedAt, evt.Kind, string(tagsj), evt.Content, evt.Sig, vectorStr)
	return err
}

func (b *OracleBackend) SemanticSearch(ctx context.Context, query string, limit int) ([]SemanticSearchResult, error) {
	queryEmbedding, err := b.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	if limit <= 0 {
		limit = 10
	}

	vectorStr := formatVector(queryEmbedding)

	rows, err := b.DB.QueryContext(ctx, `
		SELECT id, pubkey, created_at, kind, tags, content, sig,
			   1 - VECTOR_DISTANCE(embedding, :1, COSINE) AS similarity
		FROM event
		WHERE embedding IS NOT NULL
		ORDER BY VECTOR_DISTANCE(embedding, :1, COSINE)
		FETCH FIRST :2 ROWS ONLY
	`, vectorStr, limit)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to execute semantic search: %w", err)
	}
	defer rows.Close()

	var results []SemanticSearchResult
	for rows.Next() {
		var evt nostr.Event
		var timestamp int64
		var tagsJSON string
		var similarity float64

		err := rows.Scan(&evt.ID, &evt.PubKey, &timestamp,
			&evt.Kind, &tagsJSON, &evt.Content, &evt.Sig, &similarity)
		if err != nil {
			return nil, err
		}
		evt.CreatedAt = nostr.Timestamp(timestamp)
		if err := json.Unmarshal([]byte(tagsJSON), &evt.Tags); err != nil {
			return nil, err
		}

		results = append(results, SemanticSearchResult{
			Event:      &evt,
			Similarity: similarity,
		})
	}

	return results, nil
}

func (b *OracleBackend) BackfillEmbeddings(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	rows, err := b.DB.QueryContext(ctx, `
		SELECT id, content FROM event
		WHERE embedding IS NULL
		FETCH FIRST :1 ROWS ONLY
	`, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var updated int
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			return updated, err
		}

		embedding, err := b.GenerateEmbedding(ctx, content)
		if err != nil {
			continue
		}

		if err := b.UpdateEventEmbedding(ctx, id, embedding); err != nil {
			continue
		}
		updated++
	}

	return updated, nil
}

func formatVector(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
