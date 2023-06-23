package main

import (
	"crypto/md5"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Creates or updates file at <stamp> with the checksum of <path>
func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: checkstamp <stamp> <path> ...")
		os.Exit(1)
	}
	sum, err := checksum(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error calculating checksum: %v\n", err)
		os.Exit(2)
	}
	err = stamp(os.Args[1], sum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating stamp: %v\n", err)
		os.Exit(3)
	}
}

func stamp(path, sum string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ioutil.WriteFile(path, []byte(sum), 0644)
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path %s is a directory, not a file", path)
	}
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	if string(content) != sum {
		return ioutil.WriteFile(path, []byte(sum), 0644)
	}
	return nil // do nothing if the hash has not changed
}

func checksum(path string) (string, error) {
	hash := md5.New()
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		hash.Write([]byte(filepath.Join(path, info.Name())))
		if !info.IsDir() && info.Mode()&fs.ModeType != os.ModeSymlink {
			data, err := ioutil.ReadFile(p)
			if err != nil {
				return err
			}
			hash.Write(data)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
