package oracle

import (
	"sync"

	"github.com/jmoiron/sqlx"
)

type OracleBackend struct {
	sync.Mutex
	*sqlx.DB
	DatabaseURL       string
	QueryLimit        int
	QueryIDsLimit     int
	QueryAuthorsLimit int
	QueryKindsLimit   int
	QueryTagsLimit    int

	// OCI Generative AI settings for semantic search
	OCIConfigPath    string // OCI config file path, defaults to ~/.oci/config
	OCIProfile       string // OCI profile name, defaults to "DEFAULT"
	OCICompartmentID string // OCI compartment ID for Generative AI
	EmbeddingModel   string // embedding model ID, defaults to "cohere.embed-multilingual-v3.0"
}

func (b *OracleBackend) Close() {
	b.DB.Close()
}
