package fs

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"os"
	stdpath "path"
	"path/filepath"
	"strings"
	"time"

	"honnef.co/go/idxgrep/magic"
)

type opener interface {
	Open(path string) (File, error)
}

type File interface {
	io.Closer
	io.Reader

	Name() string
	setName(string)
	Readdir(count int) ([]os.FileInfo, error)
	Readdirnames(count int) ([]string, error)
	Stat() (os.FileInfo, error)
}

func mimeType(f File) (string, error) {
	stat, err := f.Stat()
	if err != nil {
		return "", err
	}
	if stat.IsDir() {
		return "inode/directory", nil
	}
	if seeker, ok := f.(io.Seeker); ok {
		buf := make([]byte, 512) // TODO(dh): use magic.SniffLen
		n, _ := io.ReadFull(f, buf)
		fmime := magic.DetectContentType(buf[:n])
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		return fmime, nil
	}
	return mime.TypeByExtension(filepath.Ext(f.Name())), nil
}

func proxy(f File) (File, error) {
outer:
	for {
		fmime, err := mimeType(f)
		if err != nil {
			return nil, fmt.Errorf("couldn't determine mime type: %s", err)
		}
		for _, proxier := range proxiers {
			proxied, ok, err := proxier.Proxy(f, fmime)
			if err != nil {
				return nil, err
			}
			if ok {
				f = proxied
				continue outer
			}
		}
		return f, nil
	}
}

func Lstat(path string) (os.FileInfo, error) {
	if !strings.Contains(path, "\x00") {
		fi, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if (fi.Mode() & os.ModeType) != 0 {
			return fi, nil
		}
	}
	f, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func Open(path string) (File, error) {
	path = stdpath.Clean(path)

	segments := strings.Split(path, "\x00")
	for i, seg := range segments {
		segments[i] = stdpath.Clean(seg)
	}
	real := segments[0]

	of, err := os.Open(real)
	if err != nil {
		return nil, err
	}
	osf := &osFile{real, of}
	nf, err := proxy(osf)
	if err != nil {
		of.Close()
		return nil, err
	}
	if nf != osf {
		fi, err := nf.Stat()
		if err != nil {
			nf.Close()
			return nil, err
		}
		if fi.IsDir() {
			nf.setName(nf.Name() + "\x00")
		}
	}

	if len(segments) == 1 {
		return nf, err
	}
	virtual := segments[1:]

	for _, el := range virtual {
		dir, ok := nf.(opener)
		if !ok {
			return nil, errors.New("not a directory")
		}
		nnf, err := dir.Open(el)
		if err != nil {
			return nil, err
		}
		nf, err = proxy(nnf)
		if err != nil {
			return nil, err
		}
		if nf != nnf {
			fi, err := nf.Stat()
			if err != nil {
				nf.Close()
				return nil, err
			}
			if fi.IsDir() {
				nf.setName(nf.Name() + "\x00")
			}
		}
	}

	return nf, nil
}

var proxiers = []Proxier{
	GzipProxy{},
	ZipProxy{},
}

type osFile struct {
	path string
	*os.File
}

func (f *osFile) Name() string     { return f.path }
func (f *osFile) setName(s string) { f.path = s }

type Proxier interface {
	Proxy(f File, mime string) (File, bool, error)
}

type gzipFile struct {
	path       string
	r          *gzip.Reader
	underlying File
}

func (f *gzipFile) Name() string        { return f.path }
func (f *gzipFile) setName(name string) { f.path = name }

func (f *gzipFile) Close() error {
	err1 := f.r.Close()
	err2 := f.underlying.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func (f *gzipFile) Read(b []byte) (int, error) {
	return f.r.Read(b)
}

func (f *gzipFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, errors.New("not a directory")
}
func (f *gzipFile) Readdirnames(count int) ([]string, error) {
	return nil, errors.New("not a directory")
}

func (f *gzipFile) Stat() (os.FileInfo, error) { return f.underlying.Stat() }

type GzipProxy struct{}

func (GzipProxy) Proxy(f File, mime string) (File, bool, error) {
	if mime != "application/x-gzip" {
		return nil, false, nil
	}
	r, err := gzip.NewReader(f)
	if err != nil {
		return nil, true, err
	}
	return &gzipFile{
		path:       f.Name(),
		r:          r,
		underlying: f,
	}, true, nil
}

type fileInfo struct {
	name    string
	mode    os.FileMode
	modTime time.Time
}

func (fi fileInfo) Name() string       { return fi.name }
func (fi fileInfo) Size() int64        { return 0 }
func (fi fileInfo) Mode() os.FileMode  { return fi.mode }
func (fi fileInfo) ModTime() time.Time { return fi.modTime }
func (fi fileInfo) IsDir() bool        { return true }
func (fi fileInfo) Sys() interface{}   { return nil }

type zipFile struct {
	path string
	io.ReadCloser
	f   *zip.File
	zip *zip.Reader
}

func (f *zipFile) Name() string        { return f.path }
func (f *zipFile) setName(name string) { f.path = name }

func (f *zipFile) Readdir(count int) ([]os.FileInfo, error) { panic("not implemented") }
func (f *zipFile) Readdirnames(count int) ([]string, error) {
	cnt := strings.Count(f.f.Name, "/")
	var out []string
	for _, zf := range f.zip.File {
		if len(zf.Name) > len(f.f.Name) &&
			strings.HasPrefix(zf.Name, f.f.Name) &&
			(strings.Count(zf.Name, "/") == cnt ||
				(strings.Count(zf.Name, "/") == cnt+1 && zf.FileInfo().IsDir())) {

			out = append(out, zf.FileInfo().Name())
		}
	}
	return out, nil
}

func (f *zipFile) Stat() (os.FileInfo, error) {
	return f.f.FileInfo(), nil
}

type zipArchive struct {
	path       string
	r          *zip.Reader
	underlying File
}

func (f *zipArchive) Name() string        { return f.path }
func (f *zipArchive) setName(name string) { f.path = name }

func (f *zipArchive) Close() error {
	return f.underlying.Close()
}

func (f *zipArchive) Read(b []byte) (int, error) { return 0, io.EOF }

func (f *zipArchive) Readdir(count int) ([]os.FileInfo, error) {
	panic("not implemented")
}

func (f *zipArchive) Readdirnames(count int) ([]string, error) {
	var out []string
loop:
	for _, zf := range f.r.File {
		if count > 0 && len(out) == count {
			break
		}
		switch strings.Count(zf.Name, "/") {
		case 0:
		case 1:
			if !zf.Mode().IsDir() {
				continue loop
			}
		default:
			continue loop
		}
		out = append(out, "\x00"+zf.FileInfo().Name())
	}
	return out, nil
}

func (f *zipArchive) Stat() (os.FileInfo, error) {
	fi, err := f.underlying.Stat()
	if err != nil {
		return nil, err
	}
	return fileInfo{
		name:    fi.Name(),
		mode:    fi.Mode() | os.ModeDir,
		modTime: fi.ModTime(),
	}, nil
}

func (f *zipArchive) Open(path string) (File, error) {
	if len(path) == 0 {
		return nil, errors.New("invalid argument")
	}

	if path[0] == '/' {
		path = path[1:]
	}

	path = stdpath.Clean(path)
	for _, zf := range f.r.File {
		if stdpath.Clean(zf.Name) == path {
			r, err := zf.Open()
			if err != nil {
				return nil, err
			}
			return &zipFile{path: stdpath.Join(f.path, path), ReadCloser: r, f: zf, zip: f.r}, nil
		}
	}
	// XXX return an error that os.IsNotFound understands
	return nil, errors.New("file not found")
}

type ZipProxy struct{}

func (ZipProxy) Proxy(f File, mime string) (File, bool, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if mime != "application/zip" {
		return nil, false, nil
	}

	if rAt, ok := f.(io.ReaderAt); ok {
		r, err := zip.NewReader(rAt, fi.Size())
		if err != nil {
			return nil, true, err
		}

		return &zipArchive{path: f.Name(), r: r, underlying: f}, true, nil
	}
	if fi.Size() < 1024*1024*10 {
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, true, err
		}
		r, err := zip.NewReader(bytes.NewReader(b), fi.Size())
		if err != nil {
			return nil, true, err
		}
		return &zipArchive{
			path:       f.Name(),
			r:          r,
			underlying: f,
		}, true, nil
	}
	return nil, false, nil
}
