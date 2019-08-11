package main

import (
	"errors"
	"io"
)

var errRead = errors.New("cannot read on fake write store")

type fakeWriteStore struct {
}

func (s *fakeWriteStore) List() (names []string, err error) {
	return nil, errRead
}

func (s *fakeWriteStore) Open(path string) (r io.ReadCloser, err error) {
	return nil, errRead
}

type fakeWriteCloser struct {
}

var _ io.WriteCloser = &fakeWriteCloser{}

func (f *fakeWriteCloser) Write(buf []byte) (n int, err error) {
	return len(buf), nil
}

func (f *fakeWriteCloser) Close() error {
	return nil
}

// Create returns a fake WriteCloser that allows all writes and close without saving anything.
func (s *fakeWriteStore) Create(path string) (w io.WriteCloser, err error) {
	return &fakeWriteCloser{}, nil
}

// Rename fakes a successful rename.
func (s *fakeWriteStore) Rename(opath, npath string) (err error) {
	return nil
}

// Rename fakes a successful delete.
func (s *fakeWriteStore) Delete(path string) (err error) {
	return nil
}
