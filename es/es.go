package es

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
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

type ByQueryResponse struct {
	// Amount of time from start to end of the whole operation
	Took time.Duration
	// Set to true if any of the requests executed during the delete
	// by query execution have timed out.
	TimedOut bool
	// The number of documents that were successfully processed.
	Total int
	// The number of documents that were successfully deleted.
	Deleted int
	// The number of scroll responses pulled back by the delete by query.
	Batches int
	// The number of version conflicts that the delete by query hit.
	VersionConflicts int
	Noops            int
	Retries          struct {
		// The number of bulk actions retried
		Bulk int
		// The number of search actions retried
		Search int
	}
	// Amount of time the request slept to conform to
	// requests_per_second
	Throttled time.Duration
	// The number of requests per second effectively executed during
	// the delete by query.
	RequestsPerSecond float64
	ThrottledUntil    time.Time
	Failures          []string
}

func (q *ByQueryResponse) UnmarshalJSON(b []byte) error {
	var resp struct {
		Took             int  `json:"took"`
		TimedOut         bool `json:"timed_out"`
		Total            int  `json:"total"`
		Deleted          int  `json:"deleted"`
		Batches          int  `json:"batches"`
		VersionConflicts int  `json:"version_conflicts"`
		Noops            int  `json:"noops"`
		Retries          struct {
			Bulk   int `json:"bulk"`
			Search int `json:"search"`
		} `json:"retries"`
		Throttled         int      `json:"throttled"`
		RequestsPerSecond float64  `json:"requests_per_second"`
		ThrottledUntil    int      `json:"throttled_until"`
		Failures          []string `json:"failures"`
	}
	err := json.Unmarshal(b, &resp)
	if err != nil {
		return err
	}
	*q = ByQueryResponse{
		Took:             time.Duration(resp.Took) * time.Millisecond,
		TimedOut:         resp.TimedOut,
		Total:            resp.Total,
		Deleted:          resp.Deleted,
		Batches:          resp.Batches,
		VersionConflicts: resp.VersionConflicts,
		Noops:            resp.Noops,
		Retries: struct {
			Bulk   int
			Search int
		}(resp.Retries),
		Throttled:         time.Duration(resp.Throttled) * time.Millisecond,
		RequestsPerSecond: resp.RequestsPerSecond,
		// ThrottledUntil time.Time
		Failures: resp.Failures,
	}
	return nil
}

func (client *Client) DeleteByQuery(q interface{}) (*ByQueryResponse, error) {
	qq := map[string]interface{}{
		"query": q,
	}
	b, err := json.Marshal(qq)
	fmt.Println(string(b))
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(client.Base+"/files/_delete_by_query", "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// XXX handle non-200 status code
	var stats ByQueryResponse
	err = json.NewDecoder(resp.Body).Decode(&stats)
	return &stats, err
}
