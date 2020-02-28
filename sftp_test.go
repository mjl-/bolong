// +build sftp

package main

import (
	"io/ioutil"
	"testing"
)

func TestSftp(t *testing.T) {
	// Config file is in local/ with real private key for testing against a live sftp server.
	sftpConfigPath := "local/bolong-sftp.conf"
	configPath = &sftpConfigPath
	parseConfig()
	defer func() {
		configPath = nil
		config = configuration{}
	}()

	ss := store.(*sftpStore)
	names, err := ss.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("list returned names: %v", names)
	}

	_, err = ss.Open("bogus")
	if err == nil {
		t.Fatalf(`expected open of non-existent file "bogus" to return error, but got none`)
	}

	fw, err := ss.Create("file1")
	if err != nil {
		t.Fatalf(`create "file1": %v`, err)
	}

	text := "hello world!"
	_, err = fw.Write([]byte(text))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	err = fw.Close()
	if err != nil {
		t.Fatalf("close: %v", err)
	}

	names, err = ss.List()
	if err != nil {
		t.Fatalf("list after create: %v", err)
	}
	if len(names) != 1 || names[0] != "file1" {
		t.Fatalf(`list after create, expected single name "file1", saw %v`, names)
	}

	fr, err := ss.Open("file1")
	if err != nil {
		t.Fatalf(`open of "file1": %v`, err)
	}
	buf, err := ioutil.ReadAll(fr)
	if err != nil {
		t.Fatalf(`reading "file1": %v`, err)
	}
	if string(buf) != text {
		t.Fatalf(`reading "file1", expected %q, got %q`, text, string(buf))
	}
	err = fr.Close()
	if err != nil {
		t.Fatalf(`closing "file1" after reading: %v`, err)
	}

	err = ss.Rename("file1", "file2")
	if err != nil {
		t.Fatalf(`rename "file1" to "file2": %v`, err)
	}
	names, err = ss.List()
	if err != nil {
		t.Fatalf(`listing names after rename: %v`, err)
	}
	if len(names) != 1 || names[0] != "file2" {
		t.Fatalf(`names after rename, expected single "file2", saw %v`, names)
	}

	err = ss.Delete("file2")
	if err != nil {
		t.Fatalf(`delete of "file2": %v`, err)
	}

	names, err = ss.List()
	if err != nil {
		t.Fatalf(`names after delete: %v`, err)
	}
	if len(names) != 0 {
		t.Fatalf(`names after delete, expected empty list, got %v`, names)
	}

	err = ss.Close()
	if err != nil {
		t.Fatalf("closing sftpStore: %v", err)
	}
}
