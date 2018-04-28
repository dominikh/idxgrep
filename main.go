package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp/syntax"
	"sync"
	"time"

	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/internal/parser"
	"honnef.co/go/idxgrep/internal/regexp"
)

type Document struct {
	Data      string `json:"data"`
	Directory string `json:"directory"`
}

const INDEX = false
const QUERY = true

func main() {
	client := es.Client{
		Base: "http://localhost:9200",
	}
	if INDEX {
		t := time.Now()
		client.CreateIndex()

		numWorkers := 4
		ch := make(chan string)
		wg := sync.WaitGroup{}
		wg.Add(numWorkers)
		for i := 0; i < numWorkers; i++ {
			go func() {
				defer wg.Done()
				bi, err := client.BulkInsert()
				if err != nil {
					panic(err)
				}
				for path := range ch {
					b, err := ioutil.ReadFile(path)
					if err != nil {
						panic(err)
					}
					doc := Document{
						Data:      string(b),
						Directory: "file://" + filepath.Dir(path),
					}
					if err := bi.Index(doc, "file://"+path); err != nil {
						panic(err)
					}
				}
				if err := bi.Close(); err != nil {
					panic(err)
				}
			}()
		}

		f, err := os.Open("/tmp/list")
		if err != nil {
			panic(err)
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			ch <- sc.Text()
		}
		close(ch)
		wg.Wait()
		fmt.Println(time.Since(t))
	}

	if QUERY {
		re, err := syntax.Parse(`(bro|to)ken`, 0)
		if err != nil {
			panic(err)
		}
		q := parser.RegexpQuery(re)

		hits := client.Search(q)

		ree, _ := regexp.Compile(`(bro|to)ken`)
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
}
