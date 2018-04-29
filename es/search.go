package es

import (
	"bytes"
	"encoding/json"
	"net/http"

	"honnef.co/go/idxgrep/internal/parser"
)

type search struct {
	Query  boolQuery `json:"query"`
	Fields []string  `json:"stored_fields,omitempty"`
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

type SearchHit struct {
	ID     string `json:"_id"`
	Fields struct {
		Name []string `json:"name"`
		Path []string `json:"path"`
	}
}

type searchHits struct {
	Hits []SearchHit `json:"hits"`
}

type searchResult struct {
	Hits searchHits `json:"hits"`
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

func (client *Client) Search(q *parser.Query) []SearchHit {
	s := search{
		Query:  queryToES(q),
		Fields: []string{"name", "path"},
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}

	resp, err := http.Post(client.Base+"/files/_search?size=10000", "application/json", bytes.NewReader(b))
	if err != nil {
		panic(err)
	}
	// XXX check response code
	defer resp.Body.Close()

	res := searchResult{}
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		panic(err)
	}
	return res.Hits.Hits
}
