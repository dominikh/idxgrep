package main

import (
	"flag"
	"log"
	"path/filepath"
	"time"

	_ "honnef.co/go/idxgrep/cmd"
	"honnef.co/go/idxgrep/config"
	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/index"
	"honnef.co/go/idxgrep/index/chat"
	"honnef.co/go/idxgrep/index/regexp"
)

func main() {
	var (
		fVerbose bool
		fIndex   string
	)

	flag.StringVar(&fIndex, "i", "grep", "Index type")
	flag.BoolVar(&fVerbose, "v", false, "Verbose output")
	flag.Parse()
	regexp.Verbose = fVerbose

	cfg, err := config.LoadFile(config.DefaultPath)
	if err != nil {
		log.Fatalln("Error loading configuration:", err)
	}

	var idx index.Index
	client := &es.Client{
		Base: cfg.Global.Server,
	}

	switch fIndex {
	case "regexp":
		client.Index = cfg.RegexpIndex.Index
		idx = &regexp.Index{
			Client: client,
			Config: cfg.RegexpIndex,
		}
	case "weechat":
		client.Index = cfg.ChatIndex.Index
		idx = &chat.Weechat{
			Client: client,
		}
	default:
		log.Fatalln("Unknown index type", fIndex)
	}

	if err := idx.CreateIndex(); err != nil {
		log.Fatal(err)
	}

	root, err := filepath.Abs(flag.Args()[0])
	if err != nil {
		log.Fatalln("Couldn't determine absolute path:", err)
	}
	t := time.Now()
	stats, err := idx.Index(root)
	if err != nil {
		log.Fatalln("Error indexing files:", err)
	}
	log.Printf("Indexed %d and skipped %d files in %s", stats.Indexed, stats.Skipped, time.Since(t))
}
