package main

import (
	"errors"
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

	_ "honnef.co/go/idxgrep/cmd"
	"honnef.co/go/idxgrep/config"
	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/fs"
	idxregexp "honnef.co/go/idxgrep/index/regexp"
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

func queryRegexp(cfg *config.Config, opts regexOptions, verbose bool) {
	pat := "(?m)" + flag.Args()[0]
	if opts.caseInsensitive {
		pat = "(?i)" + pat
	}
	re, err := syntax.Parse(pat, syntax.Perl)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Couldn't parse regexp:", err)
	}
	q := parser.RegexpQuery(re)
	if verbose {
		log.Printf("Executing query: %s", q)
	}

	client := &es.Client{
		Base:  cfg.Global.Server,
		Index: cfg.RegexpIndex.Index,
	}
	idx := idxregexp.Index{Client: client}

	hits, err := idx.Search(q)
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
				L:      opts.listOnly,
				N:      opts.showLines,
				H:      opts.omitNames,
			}

			for path := range work {
				grep.Match = false
				f, err := fs.Open(path)
				if err != nil {
					if verbose {
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
	if verbose {
		log.Printf("Searching through %d candidate files", len(hits))
	}
	for _, hit := range hits {
		name := hit.Name
		path := filepath.Join(hit.Path, name)
		work <- path
	}
	close(work)
	wg.Wait()
	if verbose {
		log.Printf("Found matches in %d files", matchedFiles)
	}
}

type regexOptions struct {
	caseInsensitive bool
	listOnly        bool
	showLines       bool
	omitNames       bool
}

type chatOptions struct {
	from string
	to   string
}

type queryMode struct {
	mode string

	regex regexOptions
	chat  chatOptions
}

func (m *queryMode) String() string { return m.mode }
func (m *queryMode) Set(s string) error {
	switch s {
	case "regexp":
		flag.BoolVar(&m.regex.caseInsensitive, "q.i", false, "Case insensitive matching")
		flag.BoolVar(&m.regex.listOnly, "q.l", false, "List matching files only")
		flag.BoolVar(&m.regex.showLines, "q.n", false, "Show line numbers")
		flag.BoolVar(&m.regex.omitNames, "q.h", false, "Omit file names")
	case "chat":
		flag.StringVar(&m.chat.from, "q.from", "", "")
	default:
		return errors.New("unknown query mode")
	}
	m.mode = s
	return nil
}

func main() {
	usage := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [OPTION]... PATTERN\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.CommandLine.Usage = usage

	var fVerbose bool
	var qm queryMode
	flag.Var(&qm, "q", "")
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

	switch qm.mode {
	case "regexp":
		queryRegexp(cfg, qm.regex, fVerbose)
	case "chat":
		panic("not implemented")
	default:
		os.Exit(2)
	}
}
