package store

import (
	"flag"

	"github.com/dgraph-io/badger/v2"
)

// TODO: improve the configuration story here
var (
	dataDir = flag.String("data_dir", "/tmp/badger", "directory for database storage")
)

type Config struct {
	DB badger.Options
}

func DefaultConfig() Config {
	return Config{
		DB: badger.DefaultOptions(*dataDir),
	}
}
