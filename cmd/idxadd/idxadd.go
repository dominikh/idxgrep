package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"honnef.co/go/idxgrep/classify"
	"honnef.co/go/idxgrep/config"
	"honnef.co/go/idxgrep/es"
)

func main() {
	cfg, err := config.LoadFile(config.DefaultPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error loading configuration:", err)
		os.Exit(1)
	}
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
				b, err := ioutil.ReadFile(path)
				if err != nil {
					panic(err)
				}
				n := len(b)
				if n > 4096 {
					n = 4096
				}
				if classify.IsBinary(b[:n]) {
					fmt.Printf("Skipping %q because it seems to be a binary file\n", path)
					continue
				}
				fmt.Printf("Indexing %q\n", path)
				doc := es.Document{
					Data: string(b),
					Name: filepath.Base(path),
					Path: filepath.Dir(path),
				}
				id := sha256.Sum256([]byte(path))
				if err := bi.Index(doc, hex.EncodeToString(id[:])); err != nil {
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
		if info.Mode()&os.ModeType != 0 {
			return nil
		}
		path, err = filepath.Abs(path)
		if err != nil {
			fmt.Println("Couldn't determine absolute path:", err)
			return nil
		}
		if size := info.Size(); size > int64(cfg.Indexing.MaxFilesize) && cfg.Indexing.MaxFilesize > 0 {
			fmt.Printf("Skipping %q, %d bytes is larger than configured maximum of %d\n", path, size, cfg.Indexing.MaxFilesize)
			return nil
		}
		ch <- path
		return nil
	})
	close(ch)
	wg.Wait()
	fmt.Println(time.Since(t))
}
