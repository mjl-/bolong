package main

import (
	"io"
	"path/filepath"
	"sort"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type sftpStore struct {
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	remotePath string
	done       chan struct{} // Closed on Close.
}

var _ destination = &sftpStore{}

func (s *sftpStore) List() (names []string, err error) {
	l, err := s.sftpClient.ReadDir(s.remotePath)
	if err != nil {
		return nil, err
	}
	names = make([]string, len(l))
	for i, fi := range l {
		names[i] = fi.Name()
	}
	sort.Slice(names, func(i, j int) bool {
		return names[i] < names[j]
	})
	return names, nil
}

func (s *sftpStore) Open(path string) (r io.ReadCloser, err error) {
	return s.sftpClient.Open(filepath.Join(s.remotePath, path))
}

func (s *sftpStore) Create(path string) (w io.WriteCloser, err error) {
	return s.sftpClient.Create(filepath.Join(s.remotePath, path))
}

func (s *sftpStore) Rename(opath, npath string) (err error) {
	return s.sftpClient.Rename(filepath.Join(s.remotePath, opath), filepath.Join(s.remotePath, npath))
}

func (s *sftpStore) Delete(path string) (err error) {
	return s.sftpClient.Remove(filepath.Join(s.remotePath, path))
}

func (s *sftpStore) Ping() error {
	_, err := s.sftpClient.Getwd()
	return err
}

func (s *sftpStore) Close() error {
	err := s.sftpClient.Close()
	err2 := s.sshClient.Close()
	if err != nil {
		return err
	}
	close(s.done)
	return err2
}
