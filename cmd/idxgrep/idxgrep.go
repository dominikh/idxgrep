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
	"honnef.co/go/idxgrep/index/chat"
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

func queryRegexp(cfg *config.Config, opts regexOptions) {
	pat := "(?m)" + opts.message
	if opts.caseInsensitive {
		pat = "(?i)" + pat
	}
	re, err := syntax.Parse(pat, syntax.Perl)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Couldn't parse regexp:", err)
	}
	q := parser.RegexpQuery(re)
	if opts.verbose {
		log.Printf("Executing query: %s", q)
	}

	client := &es.Client{
		Base:  cfg.Global.Server,
		Index: cfg.RegexpIndex.Index,
	}
	idx := idxregexp.Index{Client: client}

	hits, err := idx.Search(q, opts.count)
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
					if opts.verbose {
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
	if opts.verbose {
		log.Printf("Searching through %d candidate files", len(hits))
	}
	for _, hit := range hits {
		name := hit.Name
		path := filepath.Join(hit.Path, name)
		work <- path
	}
	close(work)
	wg.Wait()
	if opts.verbose {
		log.Printf("Found matches in %d files", matchedFiles)
	}
}

func queryChat(cfg *config.Config, opts chatOptions) {
	client := &es.Client{
		Base:  cfg.Global.Server,
		Index: cfg.ChatIndex.Index,
	}
	idx := &chat.Index{Client: client}
	q := es.BoolQuery{}
	if opts.channel != "" {
		q.And = append(q.And, es.Match{Key: "channel_or_person", Value: opts.channel})
	}
	if opts.from != "" {
		q.And = append(q.And, es.Match{Key: "from", Value: opts.from})
	}
	if opts.protocol != "" {
		q.And = append(q.And, es.Match{Key: "protocol", Value: opts.protocol})
	}
	if opts.server != "" {
		q.And = append(q.And, es.Match{Key: "server", Value: opts.server})
	}
	if opts.message != "" {
		q.And = append(q.And, es.Match{Key: "message", Value: opts.message})
		q.Or = append(q.Or, es.Match{Key: "message.shingles", Value: opts.message})
	}

	s := es.Search{Query: q}
	msgs, err := idx.Search(s, opts.count)
	if err != nil {
		panic(err)
	}
	for _, msg := range msgs {
		fmt.Println(msg)
	}
}

type generalOptions struct {
	verbose bool
	message string
	count   int
}

type regexOptions struct {
	*generalOptions

	caseInsensitive bool
	listOnly        bool
	showLines       bool
	omitNames       bool
}

type chatOptions struct {
	*generalOptions

	from     string
	to       string
	protocol string
	server   string
	channel  string
}

type queryMode struct {
	mode string

	general generalOptions
	regex   regexOptions
	chat    chatOptions
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
		flag.StringVar(&m.chat.protocol, "q.protocol", "", "")
		flag.StringVar(&m.chat.server, "q.server", "", "")
		flag.StringVar(&m.chat.channel, "q.channel", "", "")
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

	var qm queryMode
	qm.regex.generalOptions = &qm.general
	qm.chat.generalOptions = &qm.general
	flag.Var(&qm, "q", "")
	flag.BoolVar(&qm.general.verbose, "v", false, "Verbose output")
	flag.IntVar(&qm.general.count, "n", 10, "Max number of results")
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	if err := flag.CommandLine.Parse(os.Args[1:]); err == flag.ErrHelp {
		os.Exit(2)
	}

	if flag.NArg() > 1 {
		flag.CommandLine.Usage()
		os.Exit(2)
	}

	if flag.NArg() > 0 {
		qm.general.message = flag.Arg(0)
	}

	cfg, err := config.LoadFile(config.DefaultPath)
	if err != nil {
		log.Fatalln("Error loading configuration:", err)
	}

	switch qm.mode {
	case "regexp":
		queryRegexp(cfg, qm.regex)
	case "chat":
		queryChat(cfg, qm.chat)
	default:
		os.Exit(2)
	}
}
