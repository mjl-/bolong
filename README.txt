e bolong

Bolong is a simple, secure and fast command-line backup and restore tool.

Features:

- Full and incremental backups. You can configure how many incremental backups
  are made before a full backup is created. Incremental backups only store files
  that have different size/mtime/permissions compared to the previous backup.
  Bolong does not compare file contents.
- Stores data either in the "local" file system (which can be a mounted network
  disk), "sftp" or "googles3" for Google's S3 storage clone (not AWS, only Google
  does reasonable streaming uploads).
- Compression with lz4. Compression rate is not too great, but it's very fast
  and won't slow restores down.
- Encrypted and authenticated data. A cloud storage provider cannot read your
  data, and cannot tamper with it.

Non-features:

- Deduplication. It would be a nice feature, but too much code/complexity for
  our purposes. Simple backups are more likely to be reliable backups.


## Examples

First create a config file named ".bolong.conf". By default, we look in the
current directory for a file by that name, then trying the parent directory, and
its parent etc, until it finds one. Run "bolong config" for an example.

Create a new backup of the current directory:

	bolong backup

If nothing is printed, it just worked. Add the -verbose flag to backup for
details. Next, list the available backups:

	bolong list

To restore the last backup:

	bolong restore path/to/restore/to

Add the "-verbose" flag to see the files being restored. You can also add
regular expressions to only restore matching files.

	bolong -path /myproject/ restore -name 20171001-230002 -verbose path/to/restore/to '\.go$'


## Compression

Bolong uses lz4 to compress all data. It is fast enough to apply it to all
files, so there is no need to complicate the code and configuration with
applying compression selectively. Decompression is also very fast, so it will
not slow down your restores.  The price you pay is a compression ratio that
isn't too great.

## Encryption

You do not want a cloud storage provider being able to read your backups. Or
tamper with them. All backed up files are encrypted, with an AEAD mode/cipher,
meaning it is also authenticated, and attempts to modify data are detected.

Your files are protected by a passphrase. Each backed up file starts with a 32
byte salt. For each file, a key is derived using PBKDF2.

## File format

Each backup is made of two files:

1. Data file, containing the contents of all files stored in this backup.
2. Index file, listing all files and meta information in this backup (file name,
regular/directory, permissions, mtime, and offset into data file (bolong doesn't
currently store owner/group). An incremental backup lists all files that would
be restored for a restore operation, not only the modified files.

Each file starts with a 32 byte salt. Followed by data in the DARE format (Data
at Rest, see https://github.com/minio/sio).

Backups, and the file names are named after the time they were initiated (in
UTC). A backup name has the form YYYYMMDD-hhmmdd. The file names have ".data"
and either ".index1.full" or ".index1.incr" appended.

## License

This software is released under an MIT license. See LICENSE.md.

## Dependencies

All dependencies are vendored in (included) in this repositories:

	https://github.com/pierrec/lz4 (BSD license)
	https://github.com/minio/sio (Apache license)
	https://github.com/mjl-/sconf (MIT license)
	https://github.com/mjl-/xfmt (MIT license)

## Contact

For feedback, contact Mechiel Lukkien at mechiel@ueber.net.


## Todo

- progress bar doesn't work as expected. we show bytes transfered of total bytes in file. but we quit earlier for partial restores. should indicate we aren't going to download the full block.
- should check created (remote) file exists just after uploading.
- add a mode where we encrypt using only a public key crypto in the config, with restores requiring the private key. we would need to keep track of more information locally (an index file) to make incremental backups.
- is our behaviour desirable when restoring to a directory that already has some files?  we currently fail when we try to create a file/directory that already exists.
- look into using google sdk for cloud storage.
