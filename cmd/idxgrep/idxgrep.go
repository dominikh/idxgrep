package main

import (
	"flag"
	"fmt"
	"os"

	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/internal/parser"
	"honnef.co/go/idxgrep/internal/regexp"
)

func main() {
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
		Base: "http://localhost:9200",
	}

	hits := client.Search(q)

	for _, hit := range hits {
		name := hit.ID
		f, err := os.Open(name[len("file://"):])
		if err != nil {
			panic(err)
		}
		grep.Reader(f, name)
		f.Close()
	}
}
