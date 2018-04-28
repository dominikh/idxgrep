package es

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

type BulkIndexer struct {
	w    io.WriteCloser
	done chan error
	size int

	url string
}

func NewBulkInserter(index string) (*BulkIndexer, error) {
	url := fmt.Sprintf("http://localhost:9200/%s/_doc/_bulk", index)
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

func (bi *BulkIndexer) Index(obj interface{}) error {
	if bi.done == nil {
		if err := bi.reset(); err != nil {
			return err
		}
	}

	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	if _, err := bi.w.Write([]byte("{\"index\": {}}\n")); err != nil {
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
