package memory

import (
	"context"
	"fmt"

	chromem "github.com/philippgille/chromem-go"
)

type Memory struct {
	db         *chromem.DB
	collection *chromem.Collection
}

func New(dir string, embFn chromem.EmbeddingFunc) (*Memory, error) {
	db, err := chromem.NewPersistentDB(dir, false)
	if err != nil {
		return nil, fmt.Errorf("memory db: %w", err)
	}
	col, err := db.GetOrCreateCollection("main", nil, embFn)
	if err != nil {
		return nil, fmt.Errorf("memory collection: %w", err)
	}
	return &Memory{db: db, collection: col}, nil
}

func (m *Memory) Index(ctx context.Context, id, text string) error {
	return m.collection.AddDocument(ctx, chromem.Document{
		ID:      id,
		Content: text,
	})
}

func (m *Memory) Search(ctx context.Context, query string, n int) ([]string, error) {
	if m.collection.Count() == 0 {
		return nil, nil
	}
	results, err := m.collection.Query(ctx, query, n, nil, nil)
	if err != nil {
		return nil, err
	}
	texts := make([]string, len(results))
	for i, r := range results {
		texts[i] = r.Content
	}
	return texts, nil
}
