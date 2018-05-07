package regexp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"honnef.co/go/idxgrep/config"
	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/filter"
	"honnef.co/go/idxgrep/fs"
	"honnef.co/go/idxgrep/index"
	"honnef.co/go/idxgrep/internal/parser"
)

var Verbose = false

type Document struct {
	Data string `json:"data"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type Index struct {
	Config config.RegexpIndex
	Client *es.Client
}

func (idx *Index) Delete(path string) (*es.ByQueryResponse, error) {
	path = strings.Replace(path, "\x00", "", -1)
	q := map[string]interface{}{
		"term": map[string]interface{}{
			"path": path,
		},
	}

	return idx.Client.DeleteByQuery(q)
}

func (idx *Index) CreateIndex() error {
	body := `
	{
	  "settings": {
	    "number_of_shards": 1,
	    "number_of_replicas": 0,
	    "analysis": {
          "tokenizer": {
            "trigram": {
              "type": "ngram",
              "min_gram": 3,
              "max_gram": 3
            },
            "path": {
              "type": "path_hierarchy",
              "delimiter": "/"
            }
          },
          "char_filter": {
            "nul_to_slash": {
              "type": "pattern_replace",
              "pattern": "\u0000",
              "replacement": ""
            }
          },
	      "analyzer": {
	        "trigram": {
	          "type": "custom",
	          "tokenizer": "trigram"
	        },
            "path": {
              "type": "custom",
              "tokenizer": "path",
              "char_filter": ["nul_to_slash"]
            }
	      }
	    }
	  },
	  "mappings": {
	    "_doc": {
	      "_source": {
	        "enabled": false
	      },
          "_all": {
            "enabled": false
          },
	      "properties": {
            "name": {
              "type": "keyword",
              "store": true
            },
	        "path": {
	          "type": "text",
              "analyzer": "path",
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

	req, err := http.NewRequest("PUT", idx.Client.Base+"/"+idx.Client.Index, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		return err
	}
	resp, err := idx.Client.Do(req)
	if err != nil {
		if err, ok := err.(es.APIError); ok {
			if err.Err.Type == "resource_already_exists_exception" {
				return nil
			}
		}
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (idx *Index) Index(root string) (index.Statistics, error) {
	numWorkers := 4
	errCh := make(chan error, numWorkers)
	workCh := make(chan fs.File)
	wg := sync.WaitGroup{}
	wg.Add(numWorkers)
	indexedTotal := make([]int, numWorkers)
	skippedTotal := make([]int, numWorkers)

	for i := 0; i < numWorkers; i++ {
		i := i
		go func() {
			defer wg.Done()
			bi := idx.Client.BulkInsert()
			indexed := 0
			skipped := 0
			for f := range workCh {
				b, err := ioutil.ReadAll(f)
				f.Close()
				if err != nil {
					skipped++
					log.Printf("Skipping %q because of read error: %s", f.Name(), err)
					continue
				}
				if Verbose {
					log.Printf("Indexing %q", f.Name())
				}
				indexed++
				doc := Document{
					Data: string(b),
					Name: filepath.Base(f.Name()),
					Path: filepath.Dir(f.Name()),
				}
				id := sha256.Sum256([]byte(f.Name()))
				if err := bi.Index(doc, hex.EncodeToString(id[:])); err != nil {
					errCh <- err
					return
				}
			}
			// XXX ensure bi is always closed
			if err := bi.Close(); err != nil {
				errCh <- err
				return
			}
			indexedTotal[i] = indexed
			skippedTotal[i] = skipped
		}()
	}

	indexed := 0
	skipped := 0

	statFilters := []filter.Stat{
		filter.SpecialFile{},
	}

	fileFilters := []filter.File{
		filter.Name{
			Names: map[string]bool{
				".git":        true,
				".svn":        true,
				".sass-cache": true,
				".yardoc":     true,
				"__MACOSX":    true,
				".DS_Store":   false,
			},
		},
		filter.Size{MaxSize: int64(idx.Config.MaxFilesize)},
		filter.Binary{},
	}

	err := fs.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Couldn't process %q: %s", path, err)
			return nil
		}

		for _, filter := range statFilters {
			drop, err := filter.Filter(info)
			if err != nil {
				log.Printf("Couldn't filter %s: %s", path, err)
				return nil
			}
			if drop {
				skipped++
				if Verbose {
					log.Printf("Filtered %q by %T", info.Name(), filter)
				}
				if info.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}

		f, err := fs.Open(path)
		if err != nil {
			log.Printf("Couldn't open %s: %s", path, err)
			return nil
		}

		for _, filter := range fileFilters {
			drop, err := filter.Filter(f)
			if err != nil {
				f.Close()
				log.Printf("Couldn't filter %s: %s", path, err)
				return nil
			}
			if drop {
				f.Close()
				skipped++
				if Verbose {
					log.Printf("Filtered %q by %T", f.Name(), filter)
				}
				if info.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}

		if !info.IsDir() {
			select {
			case workCh <- f:
			case err := <-errCh:
				return err
			}
		} else {
			f.Close()
		}
		return nil
	})

	close(workCh)
	wg.Wait()

	if err != nil {
		return index.Statistics{}, err
	}

	for _, count := range indexedTotal {
		indexed += count
	}
	for _, count := range skippedTotal {
		skipped += count
	}
	return index.Statistics{Indexed: indexed, Skipped: skipped}, nil
}

func queryToES(q *parser.Query) interface{} {
	out := es.BoolQuery{}
	switch q.Op {
	case parser.QAll:
		return map[string]interface{}{"match_all": struct{}{}}
	case parser.QNone:
		return map[string]interface{}{"match_none": struct{}{}}
	case parser.QAnd:
		for _, tri := range q.Trigram {
			out.And = append(out.And, es.Term{Key: "data", Value: tri})
		}
		for _, sq := range q.Sub {
			out.And = append(out.And, queryToES(sq))
		}
	case parser.QOr:
		for _, tri := range q.Trigram {
			out.Or = append(out.Or, es.Term{Key: "data", Value: tri})
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

type SearchHit struct {
	ID   string
	Name string
	Path string
}

func (idx *Index) Search(q *parser.Query) ([]SearchHit, error) {
	s := es.Search{
		Query:  queryToES(q),
		Fields: []string{"name", "path"},
	}
	hits, err := idx.Client.Search(s)
	if err != nil {
		return nil, err
	}

	type fields struct {
		Name []string `json:"name"`
		Path []string `json:"path"`
	}
	out := make([]SearchHit, len(hits))
	for i, hit := range hits {
		var f fields
		if err := json.Unmarshal(hit.Fields, &f); err != nil {
			return nil, err
		}
		out[i] = SearchHit{
			ID:   hit.ID,
			Name: f.Name[0],
			Path: f.Path[0],
		}
	}
	return out, nil
}
