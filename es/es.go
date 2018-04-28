package es

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

type Client struct {
	Base string
}

func (client *Client) CreateIndex() error {
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
	    "_doc": {
	      "_source": {
	        "enabled": false
	      },
          "_all": {
            "enabled": false
          },
	      "properties": {
	        "directory": {
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

	req, err := http.NewRequest("PUT", client.Base+"/files", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
	// XXX handle status code
}

type BulkIndexer struct {
	w    io.WriteCloser
	done chan error
	size int

	url string
}

func (client *Client) BulkInsert() (*BulkIndexer, error) {
	url := fmt.Sprintf("%s/files/_doc/_bulk", client.Base)
	bi := &BulkIndexer{
		url: url,
	}
	return bi, nil
}

func (bi *BulkIndexer) reset() error {
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	req, err := http.NewRequest("POST", bi.url, pr)
	req.Header.Set("Content-Type", "application/x-ndjson")
	if err != nil {
		return err
	}

	bi.w = pw
	bi.done = done
	bi.size = 0

	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- err
			return
		}
		defer resp.Body.Close()
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

	return nil
}

func (bi *BulkIndexer) Index(obj interface{}, id string) error {
	if bi.done == nil {
		if err := bi.reset(); err != nil {
			return err
		}
	}

	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	type tHdr struct {
		Index struct {
			ID string `json:"_id,omitempty"`
		} `json:"index"`
	}
	hdr := tHdr{}
	hdr.Index.ID = id
	bhdr, err := json.Marshal(hdr)
	if err != nil {
		panic(err)
	}
	if _, err := bi.w.Write(bhdr); err != nil {
		return err
	}
	if _, err := bi.w.Write([]byte{'\n'}); err != nil {
		return err
	}
	if _, err := bi.w.Write(b); err != nil {
		return err
	}
	if _, err := bi.w.Write([]byte{'\n'}); err != nil {
		return err
	}

	bi.size += len(b)
	if bi.size > 1024*1024*8 {
		return bi.Flush()
	}
	return nil
}

func (bi *BulkIndexer) Close() error {
	if bi.done == nil {
		return nil
	}
	if _, err := bi.w.Write([]byte{'\n'}); err != nil {
		_ = bi.w.Close()
		return err
	}
	if err := bi.w.Close(); err != nil {
		return err
	}
	err := <-bi.done
	bi.done = nil
	return err
}

func (bi *BulkIndexer) Flush() error {
	return bi.Close()
}
