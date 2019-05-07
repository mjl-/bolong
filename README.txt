# bolong

Bolong is a simple, secure and fast command-line backup and restore tool.

Features:
- Full and incremental backups. You can configure how many incremental
backups are made before a full backup is created. Incremental backups
only store files that have different size/mtime/permissions compared
to the previous backup. Bolong does not compare file contents.
- Stores data either in the "local" file system (which can be a
mounted network disk) or in Google's S3 storage clone (not AWS,
only Google does reasonable streaming uploads).
- Compression with lz4. Compression rate is not too great, but it's
very fast, so won't slow restores down.
- Encrypted and authenticated data. A cloud storage provider cannot
read your data, and cannot tamper with it.

Non-features:
- Deduplication. It would be a nice feature, but too much code/complexity
for our purposes. Simple backups are more likely to be reliable
backups.


## Examples

First, create a config file to your liking, named ".bolong.json".
By default, we look in the current directory a file by that name,
trying the parent directory, and its parent etc, until it finds
one. Here is an example:

	{
		"kind": "googles3",
		"googles3": {
			"accessKey": "GOOGLTEST123456789",
			"secret": "bm90IGEgcmVhbCBrZXkuIG5pY2UgdHJ5IHRob3VnaCBeXg==",
			"bucket": "your-bucket-name",
			"path": "/"
		},
		"incrementalsPerFull": 6,
		"fullKeep": 8,
		"incrementalForFullKeep": 4,
		"passphrase": "She0oghoairie2Tu"
	}

For a more complete example, see bolong-example.json.txt.  Set an
explicit path for a config file like so:

	./bolong -config your-config-file.json <cmd>...

Now we can create a new backup of the current directory:

	bolong backup

If all is well, it just worked, nothing is printed. If you are
running these commands manually, you might want to add the "-verbose"
flag. So you can see what is backed up.

Next, list the available backups:

	bolong list

Finally, we can restore one of the available backups. By default,
the latest backup is restored:

	bolong restore path/to/restore/to

Again, add the "-verbose" flag for a list of files restored. You
can also add regular expressions to only restore matching files.

	bolong -path /myproject/ restore -name 20171001-230002 -verbose path/to/restore/to '\.go$'


## Compression

Bolong uses lz4 to compress all data. It is fast enough to apply it
to all files, so there is no need to complicate the code and
configuration with applying compression selectively. Decompression
is also very fast, so it won't slow down your restores.  The price
you pay is a compression ratio that isn't too great.

## Encryption

You don't want a cloud storage provider being able to read your
backups. Or tamper with them. All backed up files are encrypted,
with an AEAD mode/cipher, meaning it is also authenticated, and
attempts to modify data are detected.

Your files are protected by a passphrase. Each backed up file starts
with a 32 byte salt. For each file, a key is derived using PBKDF2.

## File format

Each backup is made of two files:

1. Data file, containing the contents of all files stored in this backup.
2. Index file, listing all files and meta information in this backup
(file name, regular/directory, permissions, mtime, and offset into
data file (bolong doesn't currently store owner/group). An incremental
backup lists all files that would be restored for a restore operation,
not only the modified files.

Each file starts with a 32 byte salt. Followed by data in the DARE
format (Data at Rest, see https://github.com/minio/sio).

Backups, and the file names are named after the time they were
initiated (in UTC). A backup name has the form YYYYMMDD-hhmmdd. The
file names have ".data" and either ".index1.full" or ".index1.incr"
appended.

## License

This software is released under an MIT license. See LICENSE.md.

## Dependencies

All dependencies are vendored in (included) in this repositories:

	https://github.com/pierrec/lz4 (BSD license)
	https://github.com/minio/sio (Apache license)

## Contact

For feedback, contact Mechiel Lukkien at mechiel@ueber.net.


## Todo

- progress bar doesn't work as expected. we show bytes transfered of total bytes in file. but we quit easier for partial restores. should indicate we aren't going to download the full block.
- should check created (remote) file exists just after uploading.
- is our behaviour desirable when restoring to a directory that already has some files?  we currently fail when we try to create a file/directory that already exists.
- look into using google sdk for cloud storage.
