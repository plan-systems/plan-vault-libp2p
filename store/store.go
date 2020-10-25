package store

import (
	"context"

	badger "github.com/dgraph-io/badger/v2"
)

func New(ctx context.Context) (*Store, error) {

	db, err := badger.Open(badger.DefaultOptions("/tmp/badger"))

	// TODO: hack
	go func() { <-ctx.Done(); db.Close() }()

	return &Store{db: db, ctx: ctx}, err
}

type Store struct {
	db  *badger.DB
	ctx context.Context
}

func (s *Store) Read() error {
	err := s.db.View(func(txn *badger.Txn) error {
		// Your code here…
		return nil
	})
	return err
}

func (s *Store) Write() error {
	err := s.db.Update(func(txn *badger.Txn) error {
		// Your code here…
		return nil
	})
	return err
}
