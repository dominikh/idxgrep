package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp/syntax"
	"strings"
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

func index(path string, data []byte) {
	fmt.Println(len(data))
	d := file{path, string(data)}
	b, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	resp, err := http.Post("http://localhost:9200/files/basic_file/", "application/json", bytes.NewReader(b))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	fmt.Println(resp.StatusCode)
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

func main() {
	if INDEX {
		t := time.Now()
		createIndex()

		f, err := os.Open("/tmp/list")
		if err != nil {
			panic(err)
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			name := sc.Text()
			path := name
			b, err := ioutil.ReadFile(path)
			if err != nil {
				panic(err)
			}
			index(path, b)
		}
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
