package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func handleFile(path string, entry fs.DirEntry, err error) error {
	if !entry.IsDir() {
		fmt.Printf("Visited: %s\n", path)
	}
	return nil
}

func deleteEmpty(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}

func main() {
	ignore := buildIgnoreExpression(".gitignore")
	var paths []string

	filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		matched, err := regexp.MatchString(ignore, path)
		if err == nil && !matched && !entry.IsDir() {
			paths = append(paths, path)
		} else if err != nil {
			fmt.Println(err)
		}
		return nil
	})

	// steps
	// 1. walk directories finding all files that have not been excluded by .gitignore.
	// 2. take md5 hash sum of each file path.
	// 3. combine hash sums in ordered way and calculate that final hash sum.
	// 4. Do this every N seconds and run a specified command once the hash has changed.

	fmt.Println(paths)
}

func buildIgnoreExpression(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		panic("oh no!")
	}

	text := string(data)
	expressions := strings.Split(text, "\n")

	for i, value := range expressions {
		expressions[i] = strings.Replace(value, ".", "\\.", -1)
	}
	var ignoreExpression string = strings.Join(deleteEmpty(expressions), "|")

	return ignoreExpression
}
