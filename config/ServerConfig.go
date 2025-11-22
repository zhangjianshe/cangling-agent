package config

import (
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
)

// ServerConfig Config all config information can be read or wrote to a file config.toml
// sample config.toml like the following
// [server]
// port = 8080
// [repository]
// root
type ServerConfig struct {
	Port      int32  `toml:"port"`
	AgentId   string `toml:"agentId"`
	ServerUrl string `toml:"serverUrl"`
}

type Config struct {
	Server ServerConfig `toml:"server"`
}

func (c *Config) Read(fileName string) error {
	config, err := GetConfig(fileName)
	if err != nil {
		log.Fatalf("Error: %v\n", err)
		return err
	}
	*c = config
	return nil
}

// GetCurrentDirectory
func GetCurrentDirectory() (string, error) {
	// Method 1: Current Working Directory
	cwdDir, cwdErr := os.Getwd()

	// Method 2: Executable Directory
	exePath, exeErr := os.Executable()
	exeDir := filepath.Dir(exePath)

	// Method 3: Caller's File Directory
	_, filename, _, callerOk := runtime.Caller(0)
	callerDir := filepath.Dir(filename)

	// Choose the most appropriate method
	if cwdErr == nil && cwdDir != "" {
		return cwdDir, nil
	}

	if exeErr == nil && exeDir != "" {
		return exeDir, nil
	}

	if callerOk {
		return callerDir, nil
	}

	return "", fmt.Errorf("could not determine current directory")
}

// GetConfig Read a config from file
func GetConfig(fileName string) (Config, error) {
	if fileName == "" {
		currDir, err := GetCurrentDirectory()
		if err != nil {
			log.Fatalf("could not determine current directory: %v", err)
		}
		currenDirConfig := path.Join(currDir, "config.toml")
		config, err := readConfig(currenDirConfig)
		if err != nil {
			// 当前目录中不存在 config.toml
			homeDir, err := os.UserHomeDir()
			if err != nil {
				log.Fatalf("could not determine home directory: %v", err)
			}
			fileName = path.Join(homeDir, ".cangling", "config.toml")
			homeDirConfig, err := readConfig(fileName)
			if err != nil {
				log.Printf("home directory not found in config file: %v", err)
				// create one
				newConfig := createConfig()
				data, err3 := toml.Marshal(newConfig)
				if err3 != nil {
					log.Printf("Error marshalling new config: %v", err3)
				} else {
					log.Printf("create a new config file : %s", currenDirConfig)
					_ = os.WriteFile(currenDirConfig, data, 0644)
				}
				return newConfig, nil
			}
			return homeDirConfig, nil
		}
		return config, err
	} else {
		return readConfig(fileName)
	}

}

func (c *Config) Write(fileName string) error {
	err := writeConfig(fileName, c)
	if err != nil {
		log.Fatalf("Error: %v\n", err)
		return err
	}
	return nil
}

func writeConfig(fileName string, config *Config) error {
	if fileName == "" {
		currDir, err := GetCurrentDirectory()
		if err != nil {
			log.Fatalf("could not determine current directory: %v", err)
			return err
		}
		fileName = path.Join(currDir, "config.toml")
	}
	marshal, err := toml.Marshal(config)
	if err != nil {
		log.Fatalf("Error: %v\n", err)
		return err
	}
	err = os.WriteFile(fileName, marshal, 0644)
	if err != nil {
		log.Fatalf("Error: %v\n", err)
		return err
	}
	return nil
}

// read config
func readConfig(fileName string) (Config, error) {
	// read from file
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		return Config{}, err
	}
	data, err := os.ReadFile(fileName)
	if err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		return Config{}, err
	}

	var cfg Config
	err = toml.Unmarshal(data, &cfg)
	if err != nil {
		fmt.Printf("Error unmarshaling TOML: %v\n", err)
		return Config{}, err
	}
	return cfg, nil
}

func createConfig() Config {
	return Config{
		Server: ServerConfig{
			Port:    50051,
			AgentId: "",
		},
	}
}
