package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp/syntax"
	"runtime"
	"sync"
	"sync/atomic"

	"honnef.co/go/idxgrep"
	_ "honnef.co/go/idxgrep/cmd"
	"honnef.co/go/idxgrep/config"
	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/fs"
	"honnef.co/go/idxgrep/internal/parser"
	"honnef.co/go/idxgrep/internal/regexp"
)

type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *syncWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(b)
}

func main() {
	usage := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [OPTION]... PATTERN\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.CommandLine.Usage = usage

	var (
		fi       bool
		fl       bool
		fn       bool
		fh       bool
		fVerbose bool
	)
	flag.BoolVar(&fi, "i", false, "Case insensitive matching")
	flag.BoolVar(&fl, "l", false, "List matching files only")
	flag.BoolVar(&fn, "n", false, "Show line numbers")
	flag.BoolVar(&fh, "h", false, "Omit file names")
	flag.BoolVar(&fVerbose, "verbose", false, "Verbose output")
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	if err := flag.CommandLine.Parse(os.Args[1:]); err == flag.ErrHelp {
		os.Exit(2)
	}

	if flag.NArg() != 1 {
		flag.CommandLine.Usage()
		os.Exit(2)
	}

	cfg, err := config.LoadFile(config.DefaultPath)
	if err != nil {
		log.Fatalln("Error loading configuration:", err)
	}

	pat := "(?m)" + flag.Args()[0]
	if fi {
		pat = "(?i)" + pat
	}
	re, err := syntax.Parse(pat, syntax.Perl)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Couldn't parse regexp:", err)
	}
	q := parser.RegexpQuery(re)
	if fVerbose {
		log.Printf("Executing query: %s", q)
	}

	client := &es.Client{
		Base:  cfg.Global.Server,
		Index: cfg.Global.Index,
	}
	idx := idxgrep.Index{Client: client}

	hits, err := client.Search(q)
	if err != nil {
		log.Fatal(err)
	}

	n := runtime.NumCPU()
	wg := sync.WaitGroup{}
	wg.Add(n)
	work := make(chan string, n*2)
	stdout := &syncWriter{w: os.Stdout}
	stderr := &syncWriter{w: os.Stderr}
	var matchedFiles uint64
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			re, _ := regexp.Compile(pat)
			grep := regexp.Grep{
				Stdout: stdout,
				Stderr: stderr,
				Regexp: re,
				L:      fl,
				N:      fn,
				H:      fh,
			}

			for path := range work {
				grep.Match = false
				f, err := fs.Open(path)
				if err != nil {
					if fVerbose {
						log.Printf("Deleting missing file %q", path)
					}
					// OPT(dh): we can evaluate full directory trees
					// to optimize the cleaning
					idx.Delete(filepath.Dir(path))
					continue
				}
				grep.Reader(f, path)
				f.Close()
				if grep.Match {
					atomic.AddUint64(&matchedFiles, 1)
				}
			}
		}()
	}
	if fVerbose {
		log.Printf("Searching through %d candidate files", len(hits))
	}
	for _, hit := range hits {
		name := hit.Fields.Name[0]
		path := filepath.Join(hit.Fields.Path[0], name)
		work <- path
	}
	close(work)
	wg.Wait()
	if fVerbose {
		log.Printf("Found matches in %d files", matchedFiles)
	}
}
