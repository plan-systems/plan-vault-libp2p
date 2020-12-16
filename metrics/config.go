package metrics

import (
	"os"

	"github.com/apex/log"
	logcli "github.com/apex/log/handlers/cli"
	logjson "github.com/apex/log/handlers/json"
	logtext "github.com/apex/log/handlers/text"
)

type Config struct {
	LogLevel     string
	LogFormat    string
	DebugAddr    string
	DebugPort    int
	DisableDebug bool
}

// Init overrides the configuration values of the Config object with
// those passed in as flags
func (cfg *Config) Init() {

	switch cfg.LogFormat {
	case "cli":
		log.SetHandler(logcli.New(os.Stderr))
	case "json":
		log.SetHandler(logjson.New(os.Stderr))
	default:
		log.SetHandler(logtext.New(os.Stderr))
	}
}
