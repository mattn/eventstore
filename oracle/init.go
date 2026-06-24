package oracle

import (
	"github.com/fiatjaf/eventstore"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	_ "github.com/sijms/go-ora/v2"
)

const (
	queryLimit        = 100
	queryIDsLimit     = 500
	queryAuthorsLimit = 500
	queryKindsLimit   = 10
	queryTagsLimit    = 100
)

var _ eventstore.Store = (*OracleBackend)(nil)

func (b *OracleBackend) Init() error {
	var err error
	var db *sqlx.DB

	if b.DB == nil {
		db, err = sqlx.Connect("oracle", b.DatabaseURL)
		if err != nil {
			return err
		}
		b.DB = db
	}
	b.DB.SetMaxOpenConns(80)

	b.DB.Mapper = reflectx.NewMapperFunc("json", sqlx.NameMapper)

	_, err = b.DB.Exec(`
DECLARE
  cnt NUMBER;
BEGIN
  SELECT COUNT(*) INTO cnt FROM user_tables WHERE table_name = 'EVENT';
  IF cnt = 0 THEN
    EXECUTE IMMEDIATE '
      CREATE TABLE event (
        id VARCHAR2(64) NOT NULL,
        pubkey VARCHAR2(64) NOT NULL,
        created_at NUMBER NOT NULL,
        kind NUMBER NOT NULL,
        tags CLOB NOT NULL CHECK (tags IS JSON),
        content CLOB NOT NULL,
        sig VARCHAR2(128) NOT NULL,
        embedding VECTOR(1024, FLOAT64),
        CONSTRAINT event_pk PRIMARY KEY (id)
      )
    ';
    EXECUTE IMMEDIATE 'CREATE INDEX pubkeyidx ON event (pubkey)';
    EXECUTE IMMEDIATE 'CREATE INDEX timeidx ON event (created_at DESC)';
    EXECUTE IMMEDIATE 'CREATE INDEX kindidx ON event (kind)';
    EXECUTE IMMEDIATE 'CREATE INDEX kindtimeidx ON event (kind, created_at DESC)';
  END IF;
END;
`)

	if b.QueryLimit == 0 {
		b.QueryLimit = queryLimit
	}
	if b.QueryIDsLimit == 0 {
		b.QueryIDsLimit = queryIDsLimit
	}
	if b.QueryAuthorsLimit == 0 {
		b.QueryAuthorsLimit = queryAuthorsLimit
	}
	if b.QueryKindsLimit == 0 {
		b.QueryKindsLimit = queryKindsLimit
	}
	if b.QueryTagsLimit == 0 {
		b.QueryTagsLimit = queryTagsLimit
	}
	if b.EmbeddingModel == "" {
		b.EmbeddingModel = "cohere.embed-multilingual-v3.0"
	}
	if b.OCIProfile == "" {
		b.OCIProfile = "DEFAULT"
	}

	return err
}
