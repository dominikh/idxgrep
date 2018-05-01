package idxgrep

import (
	"strings"

	"honnef.co/go/idxgrep/es"
)

type Index struct {
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
