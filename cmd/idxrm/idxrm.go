package main

import (
	"log"
	"os"
	"path/filepath"

	"honnef.co/go/idxgrep"
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

	client := &es.Client{
		Base:  cfg.Global.Server,
		Index: cfg.RegexpIndex.Index,
	}

	target, err := filepath.Abs(os.Args[1])
	if err != nil {
		log.Fatalln(err)
	}

	idx := &idxgrep.Index{Client: client}
	resp, err := idx.Delete(target)
	if err != nil {
		log.Fatalln(err)
	}
	spew.Dump(resp)
}
