package internal

import (
	"errors"
	"fmt"
	"github.com/restartfu/gophig"
	"os"
)

type Config struct {
	Port           int
	DBPath         string
	BootstrapPeers []string
}

func LoadConfig(path string) (Config, error) {
	g := gophig.NewGophig[Config](path, gophig.TOMLMarshaler{}, 0666)
	cfg, err := g.LoadConf()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = g.SaveConf(DefaultConfig())
			if err != nil {
				return cfg, err
			}
		}
	}
	return cfg, nil
}

func DefaultConfig() Config {
	cfg := Config{
		Port:   8080,
		DBPath: "data/blockchain",
	}
	if len(cfg.BootstrapPeers) == 0 {
		for port := 8000; port <= 8999; port++ {
			if port == cfg.Port {
				continue
			}
			cfg.BootstrapPeers = append(cfg.BootstrapPeers, fmt.Sprintf("127.0.0.1:%d", port))
		}
	}
	return cfg
}
