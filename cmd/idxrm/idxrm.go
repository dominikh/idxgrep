package main

import (
	"os"

	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/exit"
	"honnef.co/go/idxgrep/log"
	"honnef.co/go/spew"
)

func main() {
	client := es.Client{
		Base: "http://localhost:9200",
	}

	target := os.Args[1]

	q := map[string]interface{}{
		"term": map[string]interface{}{
			"path": target,
		},
	}

	resp, err := client.DeleteByQuery(q)
	if err != nil {
		log.Fatal(exit.Unavailable, err)
	}
	spew.Dump(resp)
}
