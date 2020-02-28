package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/mjl-/sconf"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var (
	version    = "dev"
	configPath = flag.String("config", "", "path to config file")
	remotePath = flag.String("path", "", "path at remote storage, overrides config file")
	store      destination
	config     configuration
)

type configuration struct {
	Local *struct {
		Path string `sconf-doc:"Path on local file system to store backups at."`
	} `sconf:"optional" sconf-doc:"Store backups in a locally mounted file system. Specify either Local or GoogleS3."`
	GoogleS3 *struct {
		AccessKey string `sconf-doc:"Google \"interoperable\" account for accessing storage."`
		Secret    string `sconf-doc:"Password for AccessKey."`
		Bucket    string `sconf-doc:"Name of bucket to store backups in."`
		Path      string `sconf-doc:"Path in bucket to store backups in, must start ane end with a slash."`
	} `sconf:"optional" sconf-doc:"Store backups on the S3-compatible Google Cloud Storage."`
	Sftp *struct {
		Address        string   `sconf-doc:"Address of ssh server."`
		Path           string   `sconf-doc:"Path on sftp server to read/write files. Can be relative (to home directory) or absolute."`
		HostPublicKeys []string `sconf-doc:"Public keys of server, each in single-line known hosts format. E.g. [host]:22 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEAk5ddq3uFeN6pQ38xxzOhftxDu+Xp39ULmMiSdxoFo"`
		User           string   `sconf-doc:"Username to login as."`
		Password       string   `sconf:"optional" sconf-doc:"Password to login with. Either Password or PrivateKey must be non-empty."`
		PrivateKey     []string `sconf:"optional" sconf-doc:"Private key to login with, each line as string, typically starting with \"-----BEGIN OPENSSH PRIVATE KEY-----\". Either Password or PrivateKey must be non-empty."`
	} `sconf:"optional" sconf-doc:"Store backups on sftp server."`
	Include                []string `sconf:"optional" sconf-doc:"If set, whitelist of files to store when making a backup, non-matching files/directories are not backed up. Files are regular expressions. When matching, directories end with a slash, except the root directory which is represented as emtpy string. If an included file also matches an exclude rule, it is not included."`
	Exclude                []string `sconf:"optional" sconf-doc:"If set, blacklist of files not to store when making a backup. Even if the file is in the whitelist."`
	IncrementalsPerFull    int      `sconf:"optional" sconf-doc:"Number of incremental backups before making another full backup. For a weekly full backup, set this to 6."`
	FullKeep               int      `sconf-doc:"Number of full backups to keep. After a backup, older backups are removed."`
	IncrementalForFullKeep int      `sconf:"optional" sconf-doc:"Number of past full backups for which also incremental backups are stored."`
	Passphrase             string   `sconf-doc:"Used for encrypting password."`
}

func check(err error, msg string) {
	if err == nil {
		return
	}
	if msg == "" {
		log.Fatal(err)
	}
	log.Fatalf("%s: %s", msg, err)
}

func main() {
	log.SetFlags(0)
	flag.Usage = func() {
		log.Println("usage:")
		log.Println("\tbolong [flags] config")
		log.Println("\tbolong [flags] testconfig")
		log.Println("\tbolong [flags] backup [flags] [directory]")
		log.Println("\tbolong [flags] restore [flags] destination [path-regexp ...]")
		log.Println("\tbolong [flags] list")
		log.Println("\tbolong [flags] listfiles [flags]")
		log.Println("\tbolong [flags] dumpindex [name]")
		log.Println("\tbolong [flags] version")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "config":
		if len(args) != 0 {
			flag.Usage()
			os.Exit(2)
		}
		err := sconf.Describe(os.Stdout, &config)
		check(err, "describing config")
	case "testconfig":
		if len(args) != 0 {
			flag.Usage()
			os.Exit(2)
		}
		parseConfig()
		log.Printf("config OK")
	case "backup":
		parseConfig()
		// Create name from timestamp now, for simpler testcode.
		name := time.Now().UTC().Format("20060102-150405")
		backupCmd(args, name)
	case "restore":
		parseConfig()
		restoreCmd(args)
	case "list":
		parseConfig()
		list(args)
	case "listfiles":
		parseConfig()
		listfiles(args)
	case "dumpindex":
		parseConfig()
		dumpindex(args)
	case "version":
		if len(args) != 0 {
			flag.Usage()
			os.Exit(2)
		}
		fmt.Println(version)
	default:
		flag.Usage()
		os.Exit(2)
	}

	if store != nil {
		err := store.Close()
		if err != nil {
			log.Fatalf("closing destination store: %v", err)
		}
	}
}

func parseConfig() {
	if *configPath == "" {
		findConfigPath()
	}
	err := sconf.ParseFile(*configPath, &config)
	check(err, "reading config")

	configs := []string{}
	if config.Local != nil {
		configs = append(configs, "Local")
	}
	if config.GoogleS3 != nil {
		configs = append(configs, "GoogleS3")
	}
	if config.Sftp != nil {
		configs = append(configs, "Sftp")
	}
	if len(configs) != 1 {
		log.Fatalf("must have exactly one of Local, GoogleS3 or Sftp configured, saw %v", configs)
	}

	switch {
	case config.Local != nil:
		if *remotePath != "" {
			config.Local.Path = *remotePath
		}
		path := config.Local.Path
		if !strings.HasSuffix(path, "/") {
			path += "/"
		}
		store = &local{path}
	case config.GoogleS3 != nil:
		if *remotePath != "" {
			config.GoogleS3.Path = *remotePath
		}
		path := config.GoogleS3.Path
		if !strings.HasPrefix(path, "/") || !strings.HasSuffix(path, "/") {
			log.Fatal(`field "GoogleS3.Path" must start and end with a slash`)
		}
		store = &googleS3{config.GoogleS3.Bucket, path}
	case config.Sftp != nil:
		if *remotePath != "" {
			config.Sftp.Path = *remotePath
		}
		path := config.Sftp.Path
		if path != "" && !strings.HasSuffix(path, "/") {
			path += "/"
		}

		hostPublicKeys := []ssh.PublicKey{}
		for _, ks := range config.Sftp.HostPublicKeys {
			marker, _, hostPubKey, _, _, err := ssh.ParseKnownHosts([]byte(ks))
			check(err, "parsing sftp host public key in known host format: "+ks)
			if marker != "" {
				log.Fatalf("marker must be empty string, saw %q", marker)
			}
			hostPublicKeys = append(hostPublicKeys, hostPubKey)
		}
		if len(hostPublicKeys) == 0 {
			log.Fatalf("need at least one host public key, try using ssh-keyscan to gather them")
		}

		auths := 0
		var auth []ssh.AuthMethod
		if len(config.Sftp.PrivateKey) != 0 {
			auths++
			pk := ""
			for _, line := range config.Sftp.PrivateKey {
				pk += line + "\n"
			}
			signer, err := ssh.ParsePrivateKey([]byte(pk))
			check(err, "parsing ssh private key")
			auth = append(auth, ssh.PublicKeys(signer))
		}
		if config.Sftp.Password != "" {
			auths++
			auth = append(auth, ssh.Password(config.Sftp.Password))
		}
		if auths == 0 {
			log.Fatalf("must set at least one of Password and PrivateKey for Sftp")
		}

		sshConfig := &ssh.ClientConfig{
			HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				b := key.Marshal()
				for _, hk := range hostPublicKeys {
					if bytes.Equal(b, hk.Marshal()) {
						return nil
					}
				}
				return fmt.Errorf("host key mismatch")
			},
			User: config.Sftp.User,
			Auth: auth,
		}

		sshc, err := ssh.Dial("tcp", config.Sftp.Address, sshConfig)
		check(err, "new ssh connection")

		sftpc, err := sftp.NewClient(sshc)
		check(err, "new sftp connection")

		store = &sftpStore{
			sshClient:  sshc,
			sftpClient: sftpc,
			remotePath: path,
		}
	}
	if config.Passphrase == "" {
		log.Fatalln("passphrase cannot be empty")
	}
	if config.IncrementalForFullKeep > config.FullKeep && config.FullKeep > 0 {
		log.Fatalln("incrementalForFullKeep > fullKeep does not make sense")
	}
}

func findConfigPath() {
	dir, err := os.Getwd()
	check(err, "looking for config file in current working directory")
	for {
		xpath := dir + "/.bolong.conf"
		_, err := os.Stat(xpath)
		if err == nil {
			*configPath = xpath
			return
		}
		if !os.IsNotExist(err) {
			log.Fatal("cannot find a .bolong.conf up in directory hierarchy")
		}
		ndir := path.Dir(dir)
		if ndir == dir {
			log.Fatal("cannot find a .bolong.conf up in directory hierarchy")
		}
		dir = ndir
	}
}
