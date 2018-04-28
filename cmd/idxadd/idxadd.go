package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"honnef.co/go/idxgrep/es"
)

func main() {
	client := es.Client{
		Base: "http://localhost:9200",
	}
	t := time.Now()
	client.CreateIndex()

	numWorkers := 4
	ch := make(chan string)
	wg := sync.WaitGroup{}
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			bi, err := client.BulkInsert()
			if err != nil {
				panic(err)
			}
			for path := range ch {
				fmt.Println("Indexing", path)
				b, err := ioutil.ReadFile(path)
				if err != nil {
					panic(err)
				}
				doc := es.Document{
					Data:      string(b),
					Directory: "file://" + filepath.Dir(path),
				}
				if err := bi.Index(doc, "file://"+path); err != nil {
					panic(err)
				}
			}
			if err := bi.Close(); err != nil {
				panic(err)
			}
		}()
	}

	filepath.Walk(os.Args[1], func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
			return nil
		}
		if info.IsDir() {
			return nil
		}
		path, err = filepath.Abs(path)
		if err != nil {
			fmt.Println("Couldn't determine absolute path:", err)
			return nil
		}
		ch <- path
		return nil
	})
	close(ch)
	wg.Wait()
	fmt.Println(time.Since(t))
}
