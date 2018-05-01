package main

import (
	"log"
	"os"
	"path/filepath"

	_ "honnef.co/go/idxgrep/cmd"
	"honnef.co/go/idxgrep/config"
	"honnef.co/go/idxgrep/es"
	"honnef.co/go/spew"
)

func main() {
	cfg, err := config.LoadFile(config.DefaultPath)
	if err != nil {
		log.Fatalln("Error loading configuration:", err)
	}

	client := es.Client{
		Base:  cfg.Global.Server,
		Index: cfg.Global.Index,
	}

	target, err := filepath.Abs(os.Args[1])
	if err != nil {
		log.Fatalln(err)
	}
	q := map[string]interface{}{
		"term": map[string]interface{}{
			"path": target,
		},
	}

	resp, err := client.DeleteByQuery(q)
	if err != nil {
		log.Fatalln(err)
	}
	spew.Dump(resp)
}
