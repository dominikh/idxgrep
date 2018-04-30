package indexer

import (
	"io"
	"os"
	"path/filepath"

	"honnef.co/go/idxgrep/classify"
	"honnef.co/go/idxgrep/fs"
)

type StatFilter interface {
	Filter(os.FileInfo) (drop bool, err error)
}

type FileFilter interface {
	Filter(fs.File) (drop bool, err error)
}

type GitFilter struct{}

func (GitFilter) Filter(f fs.File) (drop bool, err error) {
	// TODO(dh): for now we simply filter .git directories. In the
	// future, we should index individual commits.
	if filepath.Base(f.Name()) != ".git" {
		return false, nil
	}
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	return fi.IsDir(), nil
}

type BinaryFilter struct{}

func (BinaryFilter) Filter(f fs.File) (drop bool, err error) {
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	if fi.IsDir() {
		return false, nil
	}

	rc, err := fs.Open(f.Name())
	if err != nil {
		return false, nil
	}
	defer rc.Close()
	b := make([]byte, 4096)
	n, err := io.ReadFull(rc, b)
	if err != nil {
		if err == io.EOF {
			// empty file
			return false, nil
		}
		if err != io.ErrUnexpectedEOF {
			// actual read error
			return false, err
		}
	}
	return classify.IsBinary(b[:n]), nil
}

type SizeFilter struct {
	MaxSize int64
}

func (filter SizeFilter) Filter(f fs.File) (drop bool, err error) {
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	if fi.Size() > filter.MaxSize {
		return true, nil
	}
	return false, nil
}

type SpecialFileFilter struct{}

func (SpecialFileFilter) Filter(fi os.FileInfo) (drop bool, err error) {
	if fi.IsDir() {
		return false, nil
	}
	if fi.Mode()&os.ModeType != 0 {
		return true, nil
	}
	return false, nil
}
