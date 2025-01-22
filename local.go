package main

import (
	"io"
	"os"
)

type local struct {
	path string
}

var _ destination = &local{}

// List returns filenames sorted by name.
func (l *local) List() (names []string, err error) {
	files, err := os.ReadDir(l.path)
	if err != nil {
		return nil, err
	}
	names = make([]string, len(files))
	for i, info := range files {
		names[i] = info.Name()
	}
	return names, nil
}

func (l *local) Open(path string) (r io.ReadCloser, err error) {
	return os.Open(l.path + path)
}

func (l *local) Create(path string) (w io.WriteCloser, err error) {
	return os.Create(l.path + path)
}

func (l *local) Rename(opath, npath string) (err error) {
	return os.Rename(l.path+opath, l.path+npath)
}

func (l *local) Delete(path string) (err error) {
	return os.Remove(l.path + path)
}

func (l *local) Ping() error {
	_, err := os.Stat(l.path)
	return err
}

func (l *local) Close() error {
	return nil
}
