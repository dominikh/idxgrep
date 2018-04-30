package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"io/ioutil"
	"log"
	"path/filepath"
	"sync"
	"time"

	_ "honnef.co/go/idxgrep/cmd"
	"honnef.co/go/idxgrep/config"
	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/indexer"
)

func main() {
	var (
		fVerbose bool
	)
	flag.BoolVar(&fVerbose, "v", false, "Verbose output")
	flag.Parse()

	cfg, err := config.LoadFile(config.DefaultPath)
	if err != nil {
		log.Fatalln("Error loading configuration:", err)
	}
	client := es.Client{
		Base:  cfg.Global.Server,
		Index: cfg.Global.Index,
	}
	t := time.Now()
	if err := client.CreateIndex(); err != nil {
		log.Fatal(err)
	}

	numWorkers := 4
	ch := make(chan indexer.File)
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
			for f := range ch {
				rc, err := f.Open()
				if err != nil {
					skipped++
					log.Printf("Skipping %q because of read error: %s", f.Path(), err)
					continue
				}
				b, err := ioutil.ReadAll(rc)
				if err != nil {
					skipped++
					log.Printf("Skipping %q because of read error: %s", f.Path(), err)
					continue
				}
				if fVerbose {
					log.Printf("Indexing %q", f.Path())
				}
				indexed++
				doc := es.Document{
					Data: string(b),
					Name: filepath.Base(f.Path()),
					Path: filepath.Dir(f.Path()),
				}
				id := sha256.Sum256([]byte(f.Path()))
				if err := bi.Index(doc, hex.EncodeToString(id[:])); err != nil {
					log.Fatalln("Error indexing files:", err)
				}
			}
			if err := bi.Close(); err != nil {
				log.Fatalln("Error indexing files:", err)
			}
			indexedTotal[i] = indexed
			skippedTotal[i] = skipped
		}()
	}

	indexed := 0
	skipped := 0

	m := indexer.Master{
		Processors: []indexer.Processor{
			indexer.GitProcessor{},
			indexer.DirectoryProcessor{},
			indexer.BinaryFilter{},
			indexer.SizeFilter{MaxSize: int64(cfg.Indexing.MaxFilesize)},
			indexer.SpecialFileFilter{},
		},
	}
	root, err := filepath.Abs(flag.Args()[0])
	if err != nil {
		log.Fatalln("Couldn't determine absolute path:", err)
	}
	cb := func(f indexer.File, err error) {
		if err != nil {
			log.Println("Couldn't process file:", err)
		}
		ch <- f
	}
	filtered := func(f indexer.File, proc indexer.Processor) {
		skipped++
		if fVerbose {
			log.Printf("Filtered %q by %T", f.Path(), proc)
		}
	}
	if err := m.Process(indexer.OSFile(root), cb, filtered); err != nil {
		log.Fatal("Error processing files:", err)
	}

	close(ch)
	wg.Wait()

	for _, count := range indexedTotal {
		indexed += count
	}
	for _, count := range skippedTotal {
		skipped += count
	}
	log.Printf("Indexed %d and skipped %d files in %s", indexed, skipped, time.Since(t))
}
