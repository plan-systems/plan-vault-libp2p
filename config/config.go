package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/apex/log"
	clilog "github.com/apex/log/handlers/cli"
	jsonlog "github.com/apex/log/handlers/json"
	textlog "github.com/apex/log/handlers/text"
	"github.com/mitchellh/go-homedir"

	"github.com/plan-systems/plan-vault-libp2p/metrics"
	"github.com/plan-systems/plan-vault-libp2p/p2p"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/server"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

const defaultEtcDir = "/etc/plan-vault"
const defaultDataDir = "/var/run/plan-vault"

var (
	flagConfigPath  string
	flagLogLevel    string
	flagDataDir     string
	flagKeyringDir  string
	flagPrintConfig bool
)

func init() {
	flag.StringVar(&flagConfigPath, "config", "", "configuration file")
	flag.StringVar(&flagLogLevel, "log_level", "debug", "log level")
	flag.StringVar(&flagDataDir, "data_dir", defaultDataDir,
		"directory for database")
	flag.StringVar(&flagKeyringDir, "keyring_dir", defaultEtcDir,
		"directory for keyring storage")
	flag.BoolVar(&flagPrintConfig, "print", false,
		"print the resulting configuration file and exit")
	flag.Parse()
}

type configSource struct {
	KeyDir  string         `json:"keyring_dir"`
	Store   *storeConfig   `json:"store"`
	Server  *grpcConfig    `json:"server"`
	P2P     *p2pConfig     `json:"p2p"`
	Metrics *metricsConfig `json:"debug"`
}

type storeConfig struct {
	DataDir string `json:"data_dir"`
}

type grpcConfig struct {
	BindAddr string `json:"addr"`
	Port     int    `json:"port"`
	Insecure bool   `json:"insecure"`
	TLSCert  string `json:"tls_cert_file"`
	TLSKey   string `json:"tls_key_file"`
}

type p2pConfig struct {
	MDNS     bool   `json:"mdns"`
	URI      string `json:"channel"`
	TCPAddr  string `json:"tcp_addr"` // TODO: switch these to multiaddr?
	TCPPort  int    `json:"tcp_port"`
	QUICAddr string `json:"quic_addr"`
	QUICPort int    `json:"quic_port"`
}

type metricsConfig struct {
	LogLevel     string `json:"log_level"`
	LogFormat    string `json:"log_format"`
	DebugAddr    string `json:"addr"`
	DebugPort    int    `json:"port"`
	DisableDebug bool   `json:"disable"`
}

func newConfigSource() *configSource {
	cfg := defaultConfigSrc()
	path := configFilePath()
	if path != "" {
		cfgData, err := ioutil.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			log.Fatalf("could not read config file: %v", err)
		}

		err = json.Unmarshal(cfgData, cfg)
		if err != nil {
			log.Fatalf("could not decode configuration file:", err)
		}
	}

	// TODO: in the future we might turn this into a subcommand if
	// we end up having other control subcommands built into the vault
	if flagPrintConfig {
		cfg.Print()
		os.Exit(0)
	}

	return cfg
}

func defaultConfigSrc() *configSource {

	flagKeyringDir, err := homedir.Expand(flagKeyringDir)
	if err != nil {
		log.Fatalf("could not parse keyring_dir path: %v", err)
	}
	flagDataDir, err := homedir.Expand(flagDataDir)
	if err != nil {
		log.Fatalf("could not parse data_dir path: %v", err)
	}

	src := &configSource{
		KeyDir: flagKeyringDir,
		Store: &storeConfig{
			DataDir: flagDataDir,
		},
		Server: &grpcConfig{
			BindAddr: "127.0.0.1",
			Port:     int(pb.Const_DefaultGrpcServicePort),
			Insecure: false,
			TLSCert:  filepath.Join(flagKeyringDir, "vault_cert.pem"),
			TLSKey:   filepath.Join(flagKeyringDir, "vault_key.pem"),
		},
		P2P: &p2pConfig{
			MDNS:     true,
			URI:      "/DISCOVERY", // TODO: what should this be?
			TCPAddr:  "127.0.0.1",
			TCPPort:  9051,
			QUICAddr: "127.0.0.1",
			QUICPort: 9051,
		},
		Metrics: &metricsConfig{
			LogLevel:     flagLogLevel,
			LogFormat:    "text",
			DebugAddr:    "127.0.0.1",
			DebugPort:    9052,
			DisableDebug: false,
		},
	}
	if isTTY() {
		src.Metrics.LogFormat = "cli"
	}

	return src
}

func configFilePath() string {
	path, err := homedir.Expand(flagConfigPath)
	if err != nil {
		log.Fatalf("could not parse config file path: %v", err)
	}

	if path != "" {
		if _, err := os.Stat(path); err != nil {
			// if someone passes an explicit config file path but it's
			// mising, we shouldn't try another file unexpectedly
			log.Fatalf("could not read config file: %v", err)
		}
		return path
	}

	// the other paths we try might not be there, so we handle that
	// and try the next path
	userDir, err := os.UserConfigDir()
	if err == nil {
		path = filepath.Join(userDir, "plan-vault", "vault.json")
		_, err := os.Stat(path)
		if err == nil {
			return path
		}
		if !os.IsNotExist(err) {
			log.Fatalf("could not read config file %q: %v", path, err)
		}
	}

	path = filepath.Join(defaultEtcDir, "vault.json")
	_, err = os.Stat(path)
	if err == nil {
		return path
	}
	if !os.IsNotExist(err) {
		log.Fatalf("could not read config file %q: %v", path, err)
	}
	return ""
}

func (src *configSource) Print() {
	pretty, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		log.Fatalf("could not pretty-print configuration")
	}
	fmt.Println(string(pretty))
}

// Config is just a wrapper around the constructed configs for each of
// the internal services
type Config struct {
	ServerConfig *server.Config
	StoreConfig  *store.Config
	P2PConfig    *p2p.Config
	Metrics      *metrics.Config
}

func NewConfig() *Config {
	src := newConfigSource()

	log.SetLevelFromString(src.Metrics.LogLevel)
	switch src.Metrics.LogFormat {
	case "cli":
		log.SetHandler(clilog.Default)
	case "json":
		log.SetHandler(jsonlog.Default)
	default:
		log.SetHandler(textlog.Default)
	}

	cfg := &Config{
		StoreConfig: &store.Config{
			DataDir:      src.Store.DataDir,
			HasDiscovery: src.P2P.URI != "",
			Log:          log.WithFields(log.Fields{"service": "store"}),
		},
		ServerConfig: &server.Config{
			Addr:        src.Server.BindAddr,
			Port:        src.Server.Port,
			Insecure:    src.Server.Insecure,
			TLSCertPath: src.Server.TLSCert,
			TLSKeyPath:  src.Server.TLSKey,
			Log:         log.WithFields(log.Fields{"service": "server"}),
		},
		P2PConfig: &p2p.Config{
			MDNS:     src.P2P.MDNS,
			URI:      src.P2P.URI,
			TCPAddr:  src.P2P.TCPAddr,
			TCPPort:  src.P2P.TCPPort,
			QUICAddr: src.P2P.QUICAddr,
			QUICPort: src.P2P.QUICPort,
			KeyFile:  filepath.Join(src.KeyDir, "vault.key"),
			Log:      log.WithFields(log.Fields{"service": "p2p"}),
		},
		Metrics: &metrics.Config{
			LogLevel:     src.Metrics.LogLevel,
			LogFormat:    src.Metrics.LogFormat,
			DebugAddr:    src.Metrics.DebugAddr,
			DebugPort:    src.Metrics.DebugPort,
			DisableDebug: src.Metrics.DisableDebug,
		},
	}

	cfg.StoreConfig.Init()
	cfg.ServerConfig.Init()
	cfg.P2PConfig.Init()
	cfg.Metrics.Init()
	return cfg
}

func isTTY() bool {
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
