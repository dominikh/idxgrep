package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "honnef.co/go/idxgrep/cmd"
	"honnef.co/go/idxgrep/config"
	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/internal/parser"
	"honnef.co/go/idxgrep/internal/regexp"
)

func main() {
	cfg, err := config.LoadFile(config.DefaultPath)
	if err != nil {
		log.Fatalln("Error loading configuration:", err)
	}

	grep := regexp.Grep{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	grep.AddFlags()
	insensitive := flag.Bool("i", false, "Case insensitive")
	flag.Parse()

	pat := "(?m)" + flag.Args()[0]
	if *insensitive {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Couldn't parse regexp:", err)
	}
	q := parser.RegexpQuery(re.Syntax)
	grep.Regexp = re

	client := es.Client{
		Base:  cfg.Global.Server,
		Index: cfg.Global.Index,
	}

	hits, err := client.Search(q)
	if err != nil {
		log.Fatal(err)
	}

	for _, hit := range hits {
		name := hit.Fields.Name[0]
		path := filepath.Join(hit.Fields.Path[0], name)
		f, err := os.Open(path)
		if err != nil {
			log.Println(err)
			continue
		}
		grep.Reader(f, path)
		f.Close()
	}
}
