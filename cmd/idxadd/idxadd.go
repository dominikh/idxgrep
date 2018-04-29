package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"honnef.co/go/idxgrep/classify"
	"honnef.co/go/idxgrep/config"
	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/exit"
	"honnef.co/go/idxgrep/log"
)

func main() {
	cfg, err := config.LoadFile(config.DefaultPath)
	if err != nil {
		log.Fatalln(exit.Guess(err), "Error loading configuration:", err)
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
	indexedTotal := make([]int, numWorkers)
	skippedTotal := make([]int, numWorkers)
	for i := 0; i < numWorkers; i++ {
		i := i
		go func() {
			defer wg.Done()
			bi := client.BulkInsert()
			indexed := 0
			skipped := 0
			for path := range ch {
				b, err := ioutil.ReadFile(path)
				if err != nil {
					skipped++
					log.Printf("Skipping %q because of read error: %s", path, err)
					continue
				}
				n := len(b)
				if n > 4096 {
					n = 4096
				}
				if classify.IsBinary(b[:n]) {
					skipped++
					log.Printf("Skipping %q because it seems to be a binary file", path)
					continue
				}
				log.Printf("Indexing %q", path)
				indexed++
				doc := es.Document{
					Data: string(b),
					Name: filepath.Base(path),
					Path: filepath.Dir(path),
				}
				id := sha256.Sum256([]byte(path))
				if err := bi.Index(doc, hex.EncodeToString(id[:])); err != nil {
					log.Fatalln(exit.Guess(err), "Error indexing files:", err)
				}
			}
			if err := bi.Close(); err != nil {
				log.Fatalln(exit.Guess(err), "Error indexing files:", err)
			}
			indexedTotal[i] = indexed
			skippedTotal[i] = skipped
		}()
	}

	filepath.Walk(os.Args[1], func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println(err)
			return nil
		}
		if info.Mode()&os.ModeType != 0 {
			return nil
		}
		path, err = filepath.Abs(path)
		if err != nil {
			log.Println("Couldn't determine absolute path:", err)
			return nil
		}
		if size := info.Size(); size > int64(cfg.Indexing.MaxFilesize) && cfg.Indexing.MaxFilesize > 0 {
			log.Printf("Skipping %q, %d bytes is larger than configured maximum of %d", path, size, cfg.Indexing.MaxFilesize)
			return nil
		}
		ch <- path
		return nil
	})
	close(ch)
	wg.Wait()

	indexed := 0
	skipped := 0
	for _, count := range indexedTotal {
		indexed += count
	}
	for _, count := range skippedTotal {
		skipped += count
	}
	log.Printf("Indexed %d and skipped %d files in %s", indexed, skipped, time.Since(t))
}
