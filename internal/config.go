package internal

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	Port           string
	RewardAddress  string
	DBPath         string
	BootstrapPeers []string
	NodeName       string
}

func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		Port:   "8080",
		DBPath: "data/blockchain",
	}

	file, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "port":
			cfg.Port = value
		case "reward_address":
			cfg.RewardAddress = value
		case "db_path":
			cfg.DBPath = value
		case "bootstrap_peers":
			cfg.BootstrapPeers = strings.Split(value, ",")
		case "node_name":
			cfg.NodeName = value
		}
	}

	return cfg, scanner.Err()
}
