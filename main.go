package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp/syntax"
	"strings"
	"sync"
	"time"

	"honnef.co/go/codesearch2/parser"
	"honnef.co/go/codesearch3/internal/regexp"
)

func createIndex() {
	body := `
	{
	  "settings": {
	    "number_of_shards": 1,
	    "number_of_replicas": 0,
	    "analysis": {
	      "filter": {
	        "trigram_filter": {
	          "type": "ngram",
	          "min_gram": 3,
	          "max_gram": 3
	        }
	      },
          "tokenizer": {
            "trigram": {
              "type": "ngram",
              "min_gram": 3,
              "max_gram": 3
            }
          },
	      "analyzer": {
	        "trigram": {
	          "type": "custom",
	          "tokenizer": "trigram"
	        }
	      }
	    }
	  },
	  "mappings": {
	    "basic_file": {
	      "_source": {
	        "enabled": false
	      },
          "_all": {
            "enabled": false
          },
	      "properties": {
	        "path": {
	          "type": "keyword",
	          "store": true
	        },
	        "data": {
	          "type": "text",
	          "analyzer": "trigram",
              "index_options": "docs"
	        }
	      }
	    }
	  }
	}
	`

	req, err := http.NewRequest("PUT", "http://localhost:9200/files", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		panic(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	fmt.Println(resp.StatusCode)
}

type file struct {
	Path string `json:"path"`
	Data string `json:"data"`
}

func index(w io.Writer, path string, data []byte) {
	d := file{path, string(data)}
	b, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	w.Write([]byte("{\"index\": {}}\n"))
	w.Write(b)
	w.Write([]byte{'\n'})
}

const INDEX = true
const QUERY = false

type search struct {
	Query  boolQuery `json:"query"`
	Fields []string  `json:"stored_fields"`
}

type boolQuery struct {
	And       []interface{}
	Or        []interface{}
	MinimumOr int
}

func (q boolQuery) MarshalJSON() ([]byte, error) {
	qq := struct {
		And       []interface{} `json:"must,omitempty"`
		Or        []interface{} `json:"should,omitempty"`
		MinimumOr int           `json:"minimum_should_match"`
	}(q)
	v := struct {
		Bool interface{} `json:"bool"`
	}{qq}
	return json.Marshal(v)
}

type term string

func (t term) MarshalJSON() ([]byte, error) {
	type typ struct {
		Term struct {
			Data string `json:"data"`
		} `json:"term"`
	}
	d := typ{}
	d.Term.Data = string(t)
	return json.Marshal(d)
}

func queryToES(q *parser.Query) boolQuery {
	out := boolQuery{}
	switch q.Op {
	case parser.QAll:
		panic("not implemented")
	case parser.QNone:
		panic("not implemented")
	case parser.QAnd:
		for _, tri := range q.Trigram {
			out.And = append(out.And, term(tri))
		}
		for _, sq := range q.Sub {
			out.And = append(out.And, queryToES(sq))
		}
	case parser.QOr:
		for _, tri := range q.Trigram {
			out.Or = append(out.Or, term(tri))
		}
		for _, sq := range q.Sub {
			out.Or = append(out.Or, queryToES(sq))
		}
	}
	if len(out.Or) > 0 {
		out.MinimumOr = 1
	}
	return out
}

type searchHit struct {
	Fields struct {
		Path []string `json:"path"`
	} `json:"fields"`
}

type searchHits struct {
	Hits []searchHit `json:"hits"`
}

type searchResult struct {
	Hits searchHits `json:"hits"`
}

type BulkInserter struct {
	w    io.WriteCloser
	done chan error
}

func NewBulkInserter(index, typ string) (*BulkInserter, error) {
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:9200/%s/%s/_bulk", index, typ), pr)
	req.Header.Set("Content-Type", "application/x-ndjson")
	if err != nil {
		return nil, err
	}
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- err
			return
		}
		if resp.StatusCode >= 400 {
			b, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				done <- errors.New("non-200 status code but also failed to read error from response")
				return
			}
			done <- errors.New(string(b))
			return
		}
		close(done)
	}()
	return &BulkInserter{
		w:    pw,
		done: done,
	}, nil
}

func (bi *BulkInserter) Index(obj interface{}) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	if _, err := bi.w.Write([]byte("{\"index\": {}}\n")); err != nil {
		return err
	}
	if _, err := bi.w.Write(b); err != nil {
		return err
	}
	if _, err := bi.w.Write([]byte{'\n'}); err != nil {
		return err
	}
	return nil
}

func (bi *BulkInserter) Close() error {
	if _, err := bi.w.Write([]byte{'\n'}); err != nil {
		_ = bi.w.Close()
		return err
	}
	if err := bi.w.Close(); err != nil {
		return err
	}
	return <-bi.done
}

func main() {
	if INDEX {
		t := time.Now()
		createIndex()

		numWorkers := 4
		ch := make(chan string)
		wg := sync.WaitGroup{}
		wg.Add(numWorkers)
		for i := 0; i < numWorkers; i++ {
			go func() {
				defer wg.Done()
				bi, err := NewBulkInserter("files", "basic_file")
				if err != nil {
					panic(err)
				}
				for path := range ch {
					b, err := ioutil.ReadFile(path)
					if err != nil {
						panic(err)
					}
					if err := bi.Index(file{path, string(b)}); err != nil {
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

		//fmt.Println(q)
		s := search{Query: queryToES(q), Fields: []string{"path"}}
		b, err := json.Marshal(s)
		if err != nil {
			panic(err)
		}

		resp, err := http.Post("http://localhost:9200/files/_search?size=10000", "application/json", bytes.NewReader(b))
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		res := searchResult{}
		err = json.NewDecoder(resp.Body).Decode(&res)
		if err != nil {
			panic(err)
		}

		ree, _ := regexp.Compile(`(bro|to)ken`)
		grep := regexp.Grep{
			Regexp: ree,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}
		grep.AddFlags()
		flag.Parse()
		for _, hit := range res.Hits.Hits {
			name := hit.Fields.Path[0]
			f, err := os.Open(name)
			if err != nil {
				panic(err)
			}
			grep.Reader(f, name)
			f.Close()
		}
	}
}
