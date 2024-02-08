package main

// added a comment

import (
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type DirState struct {
	hashSum   string
	fileCount int
}

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
	var intervalSeconds int
	var command string

	flag.StringVar(&command, "c", "ls", "The shell command to run.")
	flag.IntVar(&intervalSeconds, "n", 1, "The amount of in seconds time between checks.")
	flag.Parse()

	var previous DirState = checkForChanges(".")
	var current DirState = previous

	for {
		time.Sleep(1 * time.Second)
		current = checkForChanges(".")

		if current.fileCount != previous.fileCount || current.hashSum != previous.hashSum {
			fmt.Println("[FILES CHANGED RUNNING COMMAND]")
			cmdSegments := strings.Split(command, " ")
			cmd := exec.Command(cmdSegments[0], cmdSegments[1:]...)
			stdout, err := cmd.Output()

			if err != nil {
				fmt.Println(err.Error())
			} else {
				fmt.Println(string(stdout))
			}
		}

		previous = current
	}
}

func checkForChanges(dirpath string) DirState {
	ignore := buildIgnoreExpression(".gitignore")
	var paths []string

	filepath.WalkDir(dirpath, func(path string, entry fs.DirEntry, err error) error {
		matched, err := regexp.MatchString(ignore, path)
		if err == nil && !matched && !entry.IsDir() {
			paths = append(paths, path)
		} else if err != nil {
			fmt.Println(err)
		}
		return nil
	})

	hash := sha1.New()

	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		if _, err := io.Copy(hash, f); err != nil {
			log.Fatal(err)
		}
	}

	sum := hash.Sum(nil)
	hashString := hex.EncodeToString(sum)

	dirState := DirState{
		hashSum:   hashString,
		fileCount: len(paths),
	}

	return dirState
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
