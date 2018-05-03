package esregexp

import (
	"net/http"
	"strings"

	"honnef.co/go/idxgrep/es"
)

type Index struct {
	Client *es.Client
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

type Document struct {
	Data string `json:"data"`
	Name string `json:"name"`
	Path string `json:"path"`
}
