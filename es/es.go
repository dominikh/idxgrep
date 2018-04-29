package es

import (
	"fmt"
	"net/http"
	"strings"
)

type Document struct {
	Data string `json:"data"`
	Name string `json:"name"`
	Path string `json:"path"`
}

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
	      "analyzer": {
	        "trigram": {
	          "type": "custom",
	          "tokenizer": "trigram"
	        },
           "path": {
             "type": "custom",
             "tokenizer": "path"
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
	fmt.Println(resp.StatusCode)
	return nil
	// XXX handle status code
}

func (client *Client) BulkInsert() (*BulkIndexer, error) {
	url := fmt.Sprintf("%s/files/_doc/_bulk", client.Base)
	bi := &BulkIndexer{
		url: url,
	}
	return bi, nil
}
