package main

import (
	"flag"
	"os"
	"regexp/syntax"

	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/internal/parser"
	"honnef.co/go/idxgrep/internal/regexp"
)

func main() {
	client := es.Client{
		Base: "http://localhost:9200",
	}

	re, err := syntax.Parse(os.Args[1], syntax.Perl)
	if err != nil {
		panic(err)
	}
	q := parser.RegexpQuery(re)

	hits := client.Search(q)

	ree, _ := regexp.Compile(os.Args[1])
	grep := regexp.Grep{
		Regexp: ree,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	grep.AddFlags()
	flag.Parse()
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
