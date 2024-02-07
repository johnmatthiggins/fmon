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
	fmt.Println(ignore)
	filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		matched, err := regexp.MatchString(ignore, path)
		if err == nil && !matched && !entry.IsDir() {
			fmt.Printf("visited path at: %s\n", path)
		} else if err != nil {
			fmt.Println(err)
		}
		return nil
	})
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
