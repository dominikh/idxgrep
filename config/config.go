package config

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/kirsle/configdir"
	"github.com/naoina/toml"
)

type FormatError struct {
	error
}

var DefaultPath = filepath.Join(configdir.LocalConfig("idxgrep"), "idxgrep.conf")

var DefaultConfig = Config{
	Global: Global{
		Server: "http://localhost:9200",
	},
	RegexpIndex: RegexpIndex{
		Index:       "files",
		MaxFilesize: 10485760,
	},
}

type Config struct {
	Global      Global      `toml:"global"`
	RegexpIndex RegexpIndex `toml:"regexp_index"`
	ChatIndex   ChatIndex   `toml:"chat_index"`
}

type Global struct {
	Server string `toml:"server"`
}

type RegexpIndex struct {
	Index       string `toml:"index"`
	MaxFilesize int    `toml:"max_filesize"`
}

type ChatIndex struct {
	Index string `toml:"index"`
}

func Load(r io.Reader) (*Config, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	cfg := DefaultConfig
	err = toml.Unmarshal(b, &cfg)
	if err != nil {
		return nil, FormatError{err}
	}
	return &cfg, nil
}

func LoadFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	defer f.Close()
	return Load(f)
}
