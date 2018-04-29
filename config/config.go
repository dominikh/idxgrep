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

type Config struct {
	Indexing Indexing `toml:"indexing"`
}

type Indexing struct {
	MaxFilesize int `toml:"max_filesize"`
}

func Load(r io.Reader) (*Config, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var cfg Config
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
