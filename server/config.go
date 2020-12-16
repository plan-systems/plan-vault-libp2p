package server

import (
	"path/filepath"

	"github.com/apex/log"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
)

var defaultPort = int(pb.Const_DefaultGrpcServicePort)

type Config struct {
	Addr        string
	Port        int
	Insecure    bool
	TLSCertPath string
	TLSKeyPath  string
	Log         *log.Entry
}

// Init overrides the configuration values of the Config object with
// those passed in as flags
func (cfg *Config) Init() {}

func DevConfig() *Config {
	baseDir := "/tmp"
	return &Config{
		Addr:        "127.0.0.1",
		Port:        defaultPort,
		Insecure:    true,
		TLSCertPath: filepath.Join(baseDir, "vault_cert.pem"),
		TLSKeyPath:  filepath.Join(baseDir, "vault_key.pem"),
		Log:         log.WithFields(log.Fields{"service": "grpc"}),
	}
}
