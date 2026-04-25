package main

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func unTar(r io.Reader, targetBaseDir string) {
	funcName := funcName()
	tarBallReader := tar.NewReader(r)
	for {
		header, err := tarBallReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			Fatalf(funcName + "(): error while tar reader.Next() for io.Reader with targetBaseDir " + targetBaseDir + " Error: " + err.Error())
		}

		// get the individual filename and extract to the current directory
		filename := header.Name
		// check if currently extracting archive is a forge or a git module
		// we need to remove the module name from the filename otherwise the skiplist pattern would not match
		// e.g puppetlabs-stdlib-6.0.0/MAINTAINERS.md for a forge module
		// and MAINTAINERS.md for a git module
		skiplistFilename := filename
		if targetBaseDir == config.ForgeCacheDir {
			skiplistFilenameComponents := strings.SplitAfterN(filename, "/", 2)
			if len(skiplistFilenameComponents) > 1 {
				skiplistFilename = skiplistFilenameComponents[1]
			}
		}
		if matchSkiplistContent(skiplistFilename) {
			continue
		}
		targetFilename := filepath.Join(targetBaseDir, filename)

		switch header.Typeflag {
		case tar.TypeDir:
			//fmt.Println("Untarring :", targetFilename)
			// handle directory
			//fmt.Println("Creating directory :", filename)
			//err = os.MkdirAll(targetFilename, os.FileMode(header.Mode)) // or use 0755 if you prefer
			err = os.MkdirAll(targetFilename, os.FileMode(0755)) // or use 0755 if you prefer

			if err != nil {
				Fatalf(funcName + "(): error while MkdirAll() file: " + filename + " Error: " + err.Error())

			}

			err = os.Chtimes(targetFilename, header.AccessTime, header.ModTime)

			if err != nil {
				Fatalf(funcName + "(): error while Chtimes() file: " + filename + " Error: " + err.Error())

			}

		case tar.TypeReg:
			// handle normal file
			//fmt.Println("Untarring :", targetFilename)
			writer, err := os.Create(targetFilename)

			if err != nil {
				Fatalf(funcName + "(): error while Create() file: " + filename + " Error: " + err.Error())
			}
			if _, err = io.Copy(writer, tarBallReader); err != nil {
				Fatalf(funcName + "(): error while io.copy() file: " + filename + " Error: " + err.Error())
			}
			if err = os.Chmod(targetFilename, os.FileMode(header.Mode)); err != nil {
				Fatalf(funcName + "(): error while Chmod() file: " + filename + " Error: " + err.Error())
			}
			if err = os.Chtimes(targetFilename, header.AccessTime, header.ModTime); err != nil {
				Fatalf(funcName + "(): error while Chtimes() file: " + filename + " Error: " + err.Error())
			}

			_ = writer.Close()

		case tar.TypeSymlink:
			if fileExists(targetFilename) {
				if err = os.Remove(targetFilename); err != nil {
					Fatalf(funcName + "(): error while removing existing file " + targetFilename + " to be replaced with symlink pointing to " + header.Linkname + " Error: " + err.Error())
				}
			}
			if err = os.Symlink(header.Linkname, targetFilename); err != nil {
				Fatalf(funcName + "(): error while creating symlink " + targetFilename + " pointing to " + header.Linkname + " Error: " + err.Error())
			}

		case tar.TypeLink:
			if fileExists(targetFilename) {
				if err = os.Remove(targetFilename); err != nil {
					Fatalf(funcName + "(): error while removing existing file " + targetFilename + " to be replaced with hardlink pointing to " + header.Linkname + " Error: " + err.Error())
				}
			}
			if err = os.Link(header.Linkname, targetFilename); err != nil {
				Fatalf(funcName + "(): error while creating hardlink " + targetFilename + " pointing to " + header.Linkname + " Error: " + err.Error())
			}

		// Skip pax_global_header with the commit ID this archive was created from
		case tar.TypeXGlobalHeader:
			continue

		default:
			Fatalf(funcName + "(): Unable to untar type: " + string(header.Typeflag) + " in file " + filename)
		}
	}
	// tarball produced by git archive has trailing nulls in the stream which are not
	// read by the module, when removed this can cause the git archive to hang trying
	// to output the nulls into a full pipe buffer, avoid this by discarding the rest
	// until the stream ends.
	buf := make([]byte, 4096)
	nread, err := r.Read(buf)
	for nread > 0 && err == nil {
		Debugf(fmt.Sprintf("Discarded %d bytes of trailing data from tar", nread))
		nread, err = r.Read(buf)
	}
}

func matchSkiplistContent(filePath string) bool {
	for _, blPattern := range config.PurgeSkiplist {
		filepathResult, _ := filepath.Match(blPattern, filePath)
		if strings.HasPrefix(filePath, blPattern) || filepathResult {
			Debugf("skipping file " + filePath + " because purge_skiplist pattern '" + blPattern + "' matches")
			return true
		}
	}
	//Debugf("not skipping file " + filePath + " because no purge_skiplist pattern matches")
	return false
}
