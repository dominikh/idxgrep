package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "honnef.co/go/idxgrep/cmd"
	"honnef.co/go/idxgrep/config"
	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/filter"
	"honnef.co/go/idxgrep/fs"
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
	ch := make(chan fs.File)
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
				b, err := ioutil.ReadAll(f)
				f.Close()
				if err != nil {
					skipped++
					log.Printf("Skipping %q because of read error: %s", f.Name(), err)
					continue
				}
				if fVerbose {
					log.Printf("Indexing %q", f.Name())
				}
				indexed++
				doc := es.Document{
					Data: string(b),
					Name: filepath.Base(f.Name()),
					Path: filepath.Dir(f.Name()),
				}
				id := sha256.Sum256([]byte(f.Name()))
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

	statFilters := []filter.Stat{
		filter.SpecialFile{},
	}

	fileFilters := []filter.File{
		filter.Name{
			Names: map[string]bool{
				".git":      true,
				"__MACOSX":  true,
				".DS_Store": false,
			},
		},
		filter.Size{MaxSize: int64(cfg.Indexing.MaxFilesize)},
		filter.Binary{},
	}

	root, err := filepath.Abs(flag.Args()[0])
	if err != nil {
		log.Fatalln("Couldn't determine absolute path:", err)
	}
	err = fs.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Couldn't process %q: %s", path, err)
			return nil
		}

		for _, filter := range statFilters {
			drop, err := filter.Filter(info)
			if err != nil {
				log.Printf("Couldn't filter %s: %s", path, err)
				return nil
			}
			if drop {
				skipped++
				if fVerbose {
					log.Printf("Filtered %q by %T", info.Name(), filter)
				}
				if info.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}

		f, err := fs.Open(path)
		if err != nil {
			log.Printf("Couldn't open %s: %s", path, err)
			return nil
		}

		for _, filter := range fileFilters {
			drop, err := filter.Filter(f)
			if err != nil {
				f.Close()
				log.Printf("Couldn't filter %s: %s", path, err)
				return nil
			}
			if drop {
				f.Close()
				skipped++
				if fVerbose {
					log.Printf("Filtered %q by %T", f.Name(), filter)
				}
				if info.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}

		if !info.IsDir() {
			ch <- f
		} else {
			f.Close()
		}
		return nil
	})

	if err != nil {
		log.Println("Error during file system walk:", err)
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
