package p2p

import (
	"github.com/apex/log"
)

type Config struct {
	MDNS     bool
	URI      string
	TCPAddr  string
	TCPPort  int
	QUICAddr string
	QUICPort int
	KeyFile  string
	Log      *log.Entry
}

// Init overrides the configuration values of the Config object with
// those passed in as flags
func (cfg *Config) Init() {}

func DevConfig() Config {
	return Config{
		MDNS:     true,
		URI:      "/DISCOVERY",
		TCPAddr:  "127.0.0.1",
		TCPPort:  9051,
		QUICAddr: "127.0.0.1",
		QUICPort: 9051,
		KeyFile:  "/tmp/vault.key",
		Log:      log.WithFields(log.Fields{"service": "p2p"}),
	}
}
