package store

import (
	"strings"

	"github.com/apex/log"
	"github.com/dgraph-io/badger/v2"
)

type Config struct {
	DataDir      string
	DB           badger.Options
	HasDiscovery bool
	Log          *log.Entry
}

// Init overrides the configuration values of the Config object with
// those passed in as flags
func (cfg *Config) Init() {
	cfg.DB = badger.DefaultOptions(cfg.DataDir).
		WithLogger(BadgerLogger{logger: cfg.Log})
}

func DevConfig() *Config {
	dataDir := "/tmp/db"
	logger := log.WithFields(log.Fields{"service": "store"})
	return &Config{
		DataDir: dataDir,
		DB: badger.DefaultOptions(dataDir).
			WithLogger(BadgerLogger{logger: logger}),
		HasDiscovery: false,
		Log:          logger,
	}
}

// BadgerLogger wraps the our logger. The badger.Logger says it's
// "implemented by any logging system that is used for standard logs"
// but it turns out only klog implements their logs with this
// interface and klog is still missing structured fields. And badger
// adds newlines at the end of its lines for some reason
type BadgerLogger struct {
	logger *log.Entry
}

func (l BadgerLogger) Errorf(msg string, objs ...interface{}) {
	l.logger.Errorf(strings.TrimSpace(msg), objs...)
}

func (l BadgerLogger) Warningf(msg string, objs ...interface{}) {
	l.logger.Warnf(strings.TrimSpace(msg), objs...)
}

func (l BadgerLogger) Infof(msg string, objs ...interface{}) {
	l.logger.Infof(strings.TrimSpace(msg), objs...)
}

func (l BadgerLogger) Debugf(msg string, objs ...interface{}) {
	l.logger.Debugf(strings.TrimSpace(msg), objs...)
}
