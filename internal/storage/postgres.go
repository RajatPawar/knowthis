package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
	_ "github.com/lib/pq"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(databaseURL string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &PostgresStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

func (s *PostgresStore) initSchema() error {
	schema := `
		CREATE EXTENSION IF NOT EXISTS vector;
		
		CREATE TABLE IF NOT EXISTS documents (
			id VARCHAR(255) PRIMARY KEY,
			content TEXT NOT NULL,
			source VARCHAR(50) NOT NULL,
			source_id VARCHAR(255) NOT NULL,
			title VARCHAR(500),
			channel_id VARCHAR(255),
			post_id VARCHAR(255),
			user_id VARCHAR(255),
			user_name VARCHAR(255),
			timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
			content_hash VARCHAR(64) NOT NULL,
			embedding vector(1536),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
		
		CREATE INDEX IF NOT EXISTS idx_documents_content_hash ON documents(content_hash);
		CREATE INDEX IF NOT EXISTS idx_documents_source ON documents(source);
		CREATE INDEX IF NOT EXISTS idx_documents_timestamp ON documents(timestamp);
		CREATE INDEX IF NOT EXISTS idx_documents_embedding ON documents USING ivfflat (embedding vector_cosine_ops);
		
		CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_unique_content 
		ON documents(content_hash, source, source_id);
	`
	
	_, err := s.db.Exec(schema)
	return err
}

func (s *PostgresStore) StoreDocument(ctx context.Context, doc *Document) error {
	query := `
		INSERT INTO documents (
			id, content, source, source_id, title, channel_id, post_id, 
			user_id, user_name, timestamp, content_hash, embedding
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (content_hash, source, source_id) 
		DO UPDATE SET 
			content = EXCLUDED.content,
			title = EXCLUDED.title,
			updated_at = NOW()
		RETURNING id
	`
	
	var embeddingVector pgvector.Vector
	if len(doc.Embedding) > 0 {
		embeddingVector = pgvector.NewVector(doc.Embedding)
	}
	
	var id string
	err := s.db.QueryRowContext(ctx, query,
		doc.ID,
		doc.Content,
		doc.Source,
		doc.SourceID,
		doc.Title,
		doc.ChannelID,
		doc.PostID,
		doc.UserID,
		doc.UserName,
		doc.Timestamp,
		doc.ContentHash,
		embeddingVector,
	).Scan(&id)
	
	if err != nil {
		return fmt.Errorf("failed to store document: %w", err)
	}
	
	return nil
}

func (s *PostgresStore) UpdateEmbedding(ctx context.Context, documentID string, embedding []float32) error {
	query := `
		UPDATE documents 
		SET embedding = $1, updated_at = NOW() 
		WHERE id = $2
	`
	
	embeddingVector := pgvector.NewVector(embedding)
	_, err := s.db.ExecContext(ctx, query, embeddingVector, documentID)
	if err != nil {
		return fmt.Errorf("failed to update embedding: %w", err)
	}
	
	return nil
}

func (s *PostgresStore) SearchSimilar(ctx context.Context, embedding []float32, limit int) ([]*Document, error) {
	query := `
		SELECT id, content, source, source_id, title, channel_id, post_id, 
			   user_id, user_name, timestamp, content_hash, embedding,
			   1 - (embedding <=> $1) as similarity
		FROM documents 
		WHERE embedding IS NOT NULL
		ORDER BY embedding <=> $1
		LIMIT $2
	`
	
	embeddingVector := pgvector.NewVector(embedding)
	rows, err := s.db.QueryContext(ctx, query, embeddingVector, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar documents: %w", err)
	}
	defer rows.Close()
	
	var documents []*Document
	for rows.Next() {
		doc := &Document{}
		var embeddingVector pgvector.Vector
		var similarity float64
		
		err := rows.Scan(
			&doc.ID,
			&doc.Content,
			&doc.Source,
			&doc.SourceID,
			&doc.Title,
			&doc.ChannelID,
			&doc.PostID,
			&doc.UserID,
			&doc.UserName,
			&doc.Timestamp,
			&doc.ContentHash,
			&embeddingVector,
			&similarity,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan document: %w", err)
		}
		
		doc.Embedding = embeddingVector.Slice()
		doc.Similarity = similarity
		documents = append(documents, doc)
	}
	
	return documents, nil
}

func (s *PostgresStore) GetDocumentsWithoutEmbeddings(ctx context.Context, limit int) ([]*Document, error) {
	query := `
		SELECT id, content, source, source_id, title, channel_id, post_id, 
			   user_id, user_name, timestamp, content_hash
		FROM documents 
		WHERE embedding IS NULL
		ORDER BY created_at ASC
		LIMIT $1
	`
	
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get documents without embeddings: %w", err)
	}
	defer rows.Close()
	
	var documents []*Document
	for rows.Next() {
		doc := &Document{}
		
		err := rows.Scan(
			&doc.ID,
			&doc.Content,
			&doc.Source,
			&doc.SourceID,
			&doc.Title,
			&doc.ChannelID,
			&doc.PostID,
			&doc.UserID,
			&doc.UserName,
			&doc.Timestamp,
			&doc.ContentHash,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan document: %w", err)
		}
		
		documents = append(documents, doc)
	}
	
	return documents, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func HashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}