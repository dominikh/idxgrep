package es

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	Base  string
	Index string
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

	req, err := http.NewRequest("PUT", client.Base+"/"+client.Index, strings.NewReader(body))
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

type apiError struct {
	Code int
	Body []byte
}

func (err apiError) Error() string {
	return fmt.Sprintf("Status code %d\n%s", err.Code, err.Body)
}

func (client *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, apiError{resp.StatusCode, b}
	}
	return resp, nil
}

func (client *Client) BulkInsert() *BulkIndexer {
	url := fmt.Sprintf("%s/%s/_doc/_bulk", client.Base, client.Index)
	bi := &BulkIndexer{
		client: client,
		url:    url,
	}
	return bi
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

	req, err := http.NewRequest("POST", client.Base+"/"+client.Index+"/_delete_by_query", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var stats ByQueryResponse
	err = json.NewDecoder(resp.Body).Decode(&stats)
	return &stats, err
}
