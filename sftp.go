package main

import (
	"io"
	"path/filepath"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type sftpStore struct {
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	remotePath string
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

func (s *sftpStore) Close() error {
	err := s.sftpClient.Close()
	err2 := s.sshClient.Close()
	if err != nil {
		return err
	}
	return err2
}
