// Package indexer implements indexing of abstract text documents.
//
// Indexer uses a chain of processors to recursively walk file trees.
// Processors can be classified as generators and filters. Generators,
// when given a file, will produce more files. Examples include
// enumerating files in a directory or in a tar file. Filters, on the
// other hand, prevent files from being indexed. Examples include
// filtering binary files or version control directories. If no
// processors match a file, it will be indexed.
package indexer

import (
	"io"
	"os"
	"path/filepath"

	"honnef.co/go/idxgrep/classify"
)

// Two kinds of processors: generators and filters. Generators produce
// new files, filters drop files.

var _ File = OSFile("")

type OSFile string

func (f OSFile) Path() string                 { return string(f) }
func (f OSFile) Open() (io.ReadCloser, error) { return os.Open(string(f)) }
func (f OSFile) Stat() (os.FileInfo, error)   { return os.Lstat(string(f)) }
func (f OSFile) Readdir() ([]File, error) {
	of, err := os.Open(string(f))
	if err != nil {
		return nil, err
	}
	defer of.Close()
	names, err := of.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	var out []File
	for _, name := range names {
		out = append(out, OSFile(filepath.Join(string(f), name)))
	}
	return out, nil
}

type File interface {
	Path() string
	Readdir() ([]File, error)
	Stat() (os.FileInfo, error)
	Open() (io.ReadCloser, error)
}

type Processor interface {
	Process(f File) (files []File, handled bool, err error)
}

type Master struct {
	Processors []Processor
}

func (m *Master) Process(f File, index func(File, error), filtered func(File, Processor)) error {
	for _, proc := range m.Processors {
		files, handled, err := proc.Process(f)
		if err != nil {
			return err
		}
		if !handled {
			continue
		}

		if len(files) == 0 {
			if filtered != nil {
				filtered(f, proc)
			}
			return nil
		}

		for _, nf := range files {
			if err := m.Process(nf, index, filtered); err != nil {
				index(nf, err)
			}
		}
		return nil
	}

	index(f, nil)
	return nil
}

type DirectoryProcessor struct{}

func (DirectoryProcessor) Process(f File) (files []File, handled bool, err error) {
	stat, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if (stat.Mode() & os.ModeDir) == 0 {
		return nil, false, nil
	}
	files, err = f.Readdir()
	return files, true, err
}

type GitProcessor struct{}

func (GitProcessor) Process(f File) (files []File, handled bool, err error) {
	// TODO(dh): for now we simply filter .git directories. In the
	// future, we should index individual commits.
	if filepath.Base(f.Path()) != ".git" {
		return nil, false, nil
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	isDir := (fi.Mode() & os.ModeDir) != 0
	return nil, isDir, nil
}

type BinaryFilter struct{}

func (BinaryFilter) Process(f File) (files []File, handled bool, err error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if (fi.Mode() & os.ModeDir) != 0 {
		return nil, false, nil
	}

	rc, err := f.Open()
	if err != nil {
		return nil, false, nil
	}
	defer rc.Close()
	b := make([]byte, 4096)
	n, err := io.ReadFull(rc, b)
	if err != nil {
		if err == io.EOF {
			// empty file
			return nil, false, nil
		}
		if err != io.ErrUnexpectedEOF {
			// actual read error
			return nil, false, err
		}
	}
	return nil, classify.IsBinary(b[:n]), nil
}

type SizeFilter struct {
	MaxSize int64
}

func (filter SizeFilter) Process(f File) (files []File, handled bool, err error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if fi.Size() > filter.MaxSize {
		return nil, true, nil
	}
	return nil, false, nil
}

type SpecialFileFilter struct{}

func (SpecialFileFilter) Process(f File) (files []File, handled bool, err error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if fi.Mode()&os.ModeType != 0 {
		return nil, true, nil
	}
	return nil, false, nil
}
