package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/mjl-/sconf"
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
		// create name from timestamp now, for simpler testcode
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
		os.Exit(1)
	}
}

func parseConfig() {
	if *configPath == "" {
		findConfigPath()
	}
	err := sconf.ParseFile(*configPath, &config)
	check(err, "reading config")

	if config.Local == nil && config.GoogleS3 == nil {
		log.Fatal("must have either Local or GoogleS3 configured")
	}
	if config.Local != nil && config.GoogleS3 != nil {
		log.Fatal("cannot have both Local and GoogleS3 configured")
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
