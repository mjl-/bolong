package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

func backupCmd(args []string, name string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	fs.Usage = func() {
		log.Println("usage: bolong [flags] backup [flags] [directory]")
		fs.PrintDefaults()
	}
	verbose := fs.Bool("verbose", false, "print files being backed up")
	dryrun := fs.Bool("dryrun", false, "do not actually write backup, can be used with -verbose to see which files would have been written")
	fs.Parse(args)
	args = fs.Args()

	dir := "."
	switch len(args) {
	case 0:
	case 1:
		dir = args[0]
	default:
		fs.Usage()
		os.Exit(2)
	}

	includes := []*regexp.Regexp{}
	for _, s := range config.Include {
		re, err := regexp.Compile(s)
		if err != nil {
			log.Fatalf("bad include regexp %s: %s", s, err)
		}
		includes = append(includes, re)
	}
	excludes := []*regexp.Regexp{}
	for _, s := range config.Exclude {
		re, err := regexp.Compile(s)
		if err != nil {
			log.Fatalf("bad exclude regexp %s: %s", s, err)
		}
		excludes = append(excludes, re)
	}

	info, err := os.Stat(dir)
	check(err, "stat backup dir")
	if !info.IsDir() {
		log.Fatal("can only backup directories")
	}
	if dir == "." {
		dir, err = os.Getwd()
		check(err, `resolving "."`)
	}
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	// Incremental backups list the previous incr/full backups they need files from.
	// So we have to do some bookkeeping when we do an incremental backup, only keeping index files of previous backups that still have a file we need.
	type earlier struct {
		prev previous
		used bool
	}
	var earliers []earlier

	nidx := &index{}
	var oidx *index
	unseen := map[string]*file{}
	incremental := false
	if config.IncrementalsPerFull > 0 {
		// Backups will be all incremental backups (most recent first), leading to the first full backup (also included).
		backups, err := findBackupChain("latest")
		if err == nil {
			incremental = len(backups)-1 < config.IncrementalsPerFull
			if incremental {
				b := backups[0]
				oidx, err = readIndex(b)
				check(err, "parsing previous index file")
				for _, f := range oidx.contents {
					unseen[f.name] = f
				}

				earliers = make([]earlier, len(oidx.previous)+1)
				for i, p := range oidx.previous {
					earliers[i] = earlier{p, false}
				}
				earliers[len(earliers)-1] = earlier{previous{true, b.name, oidx.dataSize}, false}
			}
		} else if err == errNotFound {
			// Do first full.
		} else {
			log.Fatalln("listing backups for determing full or incremental backup:", err)
		}
	}

	// For dry run mode, we simply switch to a "store" that doesn't actually write
	// anything. We just read the previous index file above (if needed), and we don't
	// need to read any more from the store.
	if *dryrun {
		store = &fakeWriteStore{}
	}

	// Keep track of the paths we've created at remote, so we can clean up them up when we are interrupted.
	partialpaths := make(chan string)
	cleanup := make(chan os.Signal, 1)
	signal.Notify(cleanup, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		var paths []string
		cleaning := false
		for {
			select {
			case path := <-partialpaths:
				if path == "" {
					paths = nil
				} else {
					paths = append(paths, path)
				}
			case <-cleanup:
				if cleaning {
					log.Println("signal while cleaning up, quitting")
					os.Exit(1)
				}
				cleaning = true
				done := make(chan struct{})
				for _, path := range paths {
					go func(path string) {
						log.Println("cleaning up remote path", path)
						err := store.Delete(path)
						if err != nil {
							log.Println("failed to cleanup remote path:", err)
						}
						done <- struct{}{}
					}(path)
				}
				for range paths {
					<-done
				}
				os.Exit(1)
			}
		}
	}()

	dataPath := fmt.Sprintf("%s.data", name)
	var data io.WriteCloser
	data, err = store.Create(dataPath)
	check(err, "creating data file")
	partialpaths <- dataPath
	dwc := &writeCounter{f: data}
	data = dwc
	if !*dryrun {
		data, err = newSafeWriter(data)
		check(err, "creating safe file")
	}

	var whitelist []string // Whitelisted directories. All children files will be included.

	skip := func(matchPath string, matchInfo os.FileInfo, verbose bool) bool {
		if len(includes) > 0 {
			match := matchAny(includes, matchPath)
			if match && matchInfo.IsDir() {
				whitelist = append(whitelist, matchPath)
			}
			if !match && !matchInfo.IsDir() {
				keep := false
				for _, white := range whitelist {
					if strings.HasPrefix(matchPath, white) {
						keep = true
						break
					}
				}
				if !keep {
					if verbose {
						log.Println(`no "include" match, skipping`, matchPath)
					}
					return true
				}
			}
		}
		if len(excludes) > 0 {
			match := matchAny(excludes, matchPath)
			if match {
				if verbose {
					log.Println(`"exclude" match, skipping`, matchPath)
				}
				return true
			}
		}
		return false
	}

	failed := false
	dataOffset := int64(0)
	nfiles := 0
	filepath.Walk(dir, func(path string, walkInfo os.FileInfo, err error) error {
		if !strings.HasPrefix(path, dir) {
			log.Printf("path not prefixed by dir? path %s, dir %s", path, dir)
			return nil
		}

		relPath := path[len(dir):]
		matchPath := relPath
		if relPath == "" {
			relPath = "."
		}
		if walkInfo != nil && walkInfo.IsDir() && matchPath != "" {
			matchPath += "/"
		}

		if err != nil {
			// We might get a permission denied for a directory that we want to skip. So even
			// with error, check if the path was meant to be skipped.
			if walkInfo != nil && skip(matchPath, walkInfo, false) {
				return nil
			}
			if os.IsNotExist(err) || os.IsPermission(err) {
				log.Printf("error walking %s: %s, continuing with error", path, err)
				failed = true
				return nil
			}
			log.Fatalf("error walking %s: %s", path, err)
		}

		if relPath == ".bolong.conf" || strings.HasSuffix(relPath, "/.bolong.conf") {
			return nil
		}

		if skip(matchPath, walkInfo, *verbose) {
			if walkInfo.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if walkInfo.Mode()&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeCharDevice) != 0 {
			if *verbose {
				log.Printf(`skipping special file %s with mode %s`, matchPath, walkInfo.Mode())
			}
			return nil
		}

		size := int64(0)
		if !walkInfo.IsDir() {
			size = walkInfo.Size()
		}
		owner, group := userGroupName(walkInfo)
		nf := &file{
			walkInfo.IsDir(),
			walkInfo.Mode()&os.ModeSymlink != 0,
			walkInfo.Mode() & os.ModePerm,
			walkInfo.ModTime(),
			size,
			owner,
			group,
			-1, // data offset
			-1, // previous index, possibly updated later
			relPath,
		}

		nidx.contents = append(nidx.contents, nf)
		nfiles++

		if incremental {
			of, ok := unseen[relPath]
			if ok {
				delete(unseen, relPath)
				if !fileChanged(of, nf) {
					if !nf.isDir {
						nf.dataOffset = of.dataOffset
						// These indices are against the index file from the previous incremental backup.
						// We fix up these indices later on, after we know which previous backups are still referenced.
						prevIndex := of.previousIndex
						if prevIndex == -1 {
							// Files contained in the last index are now in the new previous-index-reference
							prevIndex = len(earliers) - 1
						}
						nf.previousIndex = prevIndex
						earliers[prevIndex].used = true
					}
					return nil
				}
			} else {
				nidx.add = append(nidx.add, relPath)
				if *verbose {
					fmt.Println(relPath)
				}
			}
		} else {
			if *verbose {
				fmt.Println(relPath)
			}
		}

		if nf.isDir {
			return nil
		}
		nf.dataOffset = dataOffset
		var written int64
		if nf.isSymlink {
			p, err := os.Readlink(path)
			check(err, "readlink")
			buf := []byte(p)
			n, err := data.Write(buf)
			check(err, "write symlink data")
			if n != len(buf) {
				panic("did not write full buf")
			}
			written = int64(n)
		} else {
			n, fatal, err := storeFile(path, nf.size, data)
			if err != nil {
				if !fatal {
					log.Printf("storing %s: %s, continuing with error", path, err)
					failed = true
				} else {
					log.Fatalf("storing %s: %s", path, err)
				}
			} else {
				written = n
			}
		}
		dataOffset += written

		return nil
	})

	if incremental {
		// Map previousIndex from last index file to those in index file we're making now.
		// Some old index/data files might may no longer be necessary, because all files contained within have been overwritten/deleted.
		prevIndexMap := map[int]int{}
		for i, e := range earliers {
			if e.used {
				prevIndexMap[i] = len(nidx.previous)
				nidx.previous = append(nidx.previous, e.prev)
			}
		}
		for _, f := range nidx.contents {
			if f.previousIndex >= 0 {
				f.previousIndex = prevIndexMap[f.previousIndex]
			}
		}
		for _, f := range unseen {
			nidx.delete = append(nidx.delete, f.name)
		}
		// Sort for better compression rate of index.
		sort.Slice(nidx.delete, func(i, j int) bool {
			return nidx.delete[i] < nidx.delete[j]
		})
	}

	err = data.Close()
	check(err, "closing data file")

	nidx.dataSize = dwc.size

	kind := "full"
	kindName := "full"
	if incremental {
		kind = "incr"
		kindName = "incremental"
	}
	indexPath := fmt.Sprintf("%s.index1.%s", name, kind)
	var index io.WriteCloser
	index, err = store.Create(indexPath + ".tmp")
	check(err, "creating index file")
	partialpaths <- indexPath + ".tmp"
	if !*dryrun {
		index, err = newSafeWriter(index)
		check(err, "creating safe file")
	}
	iwc := &writeCounter{f: index}
	index = iwc
	err = writeIndex(index, nidx)
	check(err, "writing index file")
	err = index.Close()
	check(err, "closing index file")
	err = store.Rename(indexPath+".tmp", indexPath)
	check(err, "moving temp index file into place")
	partialpaths <- "" // signal that we're done

	if *verbose {
		log.Printf("new %s backup: %s", kindName, name)
		addDel := ""
		if incremental {
			addDel = fmt.Sprintf(", +%d files, -%d files", len(nidx.add), len(nidx.delete))
		}
		log.Printf("total files %d, total size %s, backup size %s%s", nfiles, formatSize(dataOffset), formatSize(dwc.size+iwc.size), addDel)
	}

	if failed {
		log.Fatalf("new backup was written, but not all files could be backed up. not cleaning up old backups")
	}

	if *dryrun {
		log.Printf("note: dryrun, nothing was actually written")
		return
	}

	if config.FullKeep > 0 || config.IncrementalForFullKeep > 0 {
		backups, err := listBackups()
		check(err, "listing backups for cleaning up old backups")

		// Cleanup full backups, and everything before that.
		fullSeen := 0
		for i := len(backups) - 1; i > 0 && config.FullKeep > 0; i-- {
			if backups[i].incremental {
				continue
			}
			fullSeen++
			if fullSeen < config.FullKeep {
				continue
			}
			// Remove everything (both incr and full) before this latest full.
			for j := 0; j < i; j++ {
				ext := "full"
				kind := "full"
				if backups[j].incremental {
					ext = "incr"
					kind = "incremental"
				}
				if *verbose {
					log.Printf("cleaning up old %s backup %s", kind, backups[j].name)
				}
				err = store.Delete(backups[j].name + ".data")
				if err != nil {
					log.Println("removing old backup:", err)
				}
				err = store.Delete(backups[j].name + ".index1." + ext)
				if err != nil {
					log.Println("removing old backup:", err)
				}
			}
			// We'll continue with removing incrementals on the remaining backups, those we kept.
			backups = backups[i:]
			break
		}

		fullSeen = 0
		for i := len(backups) - 1; i > 0; i-- {
			if backups[i].incremental {
				continue
			}
			fullSeen++
			if fullSeen < config.IncrementalForFullKeep {
				continue
			}
			// Remove all incrementals before this latest full backup we've just seen.
			for j := 0; j < i; j++ {
				if !backups[j].incremental {
					continue
				}
				if *verbose {
					log.Println("cleaning up old incremental backup", backups[j].name)
				}
				err = store.Delete(backups[j].name + ".data")
				if err != nil {
					log.Println("removing old incremental backup:", err)
				}
				err = store.Delete(backups[j].name + ".index1.incr")
				if err != nil {
					log.Println("removing old incremental backup:", err)
				}
			}
			break
		}
	}
}

func matchAny(l []*regexp.Regexp, s string) bool {
	for _, re := range l {
		if re.FindStringIndex(s) != nil {
			return true
		}
	}
	return false
}

func fileChanged(old, new *file) bool {
	if old.name != new.name {
		log.Fatalf("inconsistent fileChanged call, names don't match, %s != %s", old.name, new.name)
	}
	return old.isDir != new.isDir ||
		old.isSymlink != new.isSymlink ||
		old.size != new.size ||
		old.mtime.Unix() != new.mtime.Unix() ||
		old.permissions != new.permissions ||
		old.user != new.user ||
		old.group != new.group
}

func storeFile(path string, size int64, data io.Writer) (written int64, fatal bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		nonfatal := os.IsNotExist(err) || os.IsPermission(err)
		return 0, !nonfatal, err
	}
	defer func() {
		err2 := f.Close()
		if err == nil {
			err = err2
		}
	}()
	n, err := io.Copy(data, f)
	if err != nil {
		return 0, true, err
	}
	if n < size {
		return 0, true, fmt.Errorf("expected to write %d bytes, only wrote %d", size, n)
	}
	if n > size {
		log.Printf("storing %q: file changed while storing, expected to write %d bytes, wrote %d", path, size, n)
	}
	return n, false, nil
}
