package es

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type Search struct {
	Query  interface{} `json:"query"`
	Fields []string    `json:"stored_fields,omitempty"`
}

type BoolQuery struct {
	And       []interface{}
	Or        []interface{}
	MinimumOr int
}

func (q BoolQuery) MarshalJSON() ([]byte, error) {
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

type Term struct {
	Key   string
	Value interface{}
	Boost float64
}

func (t Term) MarshalJSON() ([]byte, error) {
	type value struct {
		Value interface{} `json:"value"`
		Boost float64     `json:"boost,omitempty"`
	}

	v := struct {
		Term map[string]value `json:"term"`
	}{
		map[string]value{t.Key: value{t.Value, t.Boost}},
	}

	return json.Marshal(v)
}

type SearchHit struct {
	Index  string          `json:"_index"`
	Type   string          `json:"_type"`
	ID     string          `json:"_id"`
	Score  float64         `json:"_score"`
	Source json.RawMessage `json:"_source"`
	Fields json.RawMessage `json:"fields"`
}

type searchHits struct {
	Hits []SearchHit `json:"hits"`
}

type searchResult struct {
	Hits searchHits `json:"hits"`
}

func (client *Client) Search(s Search) ([]SearchHit, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s/_search?size=10000", client.Base, client.Index), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		if err, ok := err.(APIError); ok {
			if err.Err.Type == "index_not_found_exception" {
				return nil, nil
			}
		}
		return nil, err
	}
	defer resp.Body.Close()

	res := searchResult{}
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return nil, err
	}
	return res.Hits.Hits, nil
}
