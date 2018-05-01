package filter

import (
	"io"
	"os"
	"path/filepath"

	"honnef.co/go/idxgrep/classify"
	"honnef.co/go/idxgrep/fs"
)

type Stat interface {
	Filter(os.FileInfo) (drop bool, err error)
}

type File interface {
	Filter(fs.File) (drop bool, err error)
}

type Name struct {
	// Names to filter. The boolean specifies whether the file entry
	// has to be a directory.
	Names map[string]bool
}

func (nf Name) Filter(f fs.File) (drop bool, err error) {
	base := filepath.Base(f.Name())
	mustDir, ok := nf.Names[base]
	if !ok {
		return false, nil
	}
	if !mustDir {
		return true, nil
	}
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	return fi.IsDir(), nil
}

type Binary struct{}

func (Binary) Filter(f fs.File) (drop bool, err error) {
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

type Size struct {
	MaxSize int64
}

func (filter Size) Filter(f fs.File) (drop bool, err error) {
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	if fi.Size() > filter.MaxSize {
		return true, nil
	}
	return false, nil
}

type SpecialFile struct{}

func (SpecialFile) Filter(fi os.FileInfo) (drop bool, err error) {
	if fi.IsDir() {
		return false, nil
	}
	if fi.Mode()&os.ModeType != 0 {
		return true, nil
	}
	return false, nil
}
