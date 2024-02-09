package main

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
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
	"syscall"
	"time"
	"unicode"
)

type DirState struct {
	hashSum   string
	fileCount int
}

func main() {
	var intervalSeconds int
	var command string
	var matchRegexp string

	flag.StringVar(&command, "c", "ls", "The shell command to run.")
	flag.StringVar(&matchRegexp, "E", "", "The match expression. If a change occurs in a file that matches this regex then the command will be run.")
	flag.IntVar(&intervalSeconds, "n", 1, "The amount of in seconds time between checks.")
	flag.Parse()

	// If this is false, then we will assume that it is an ignore expression.
	var useIgnoreFile bool = matchRegexp == ""

	if useIgnoreFile {
		ignoreExpressions, err := parseIgnore(".gitignore")
		if err != nil {
			log.Fatal(err)
		}

		matchFn := func(path string) bool {
			return gitIgnoreMatch(ignoreExpressions, path)
		}
		waitForChanges(command, matchFn)
	} else {
		matchFn := func(path string) bool {
			match, err := regexp.MatchString(matchRegexp, path)
			if err != nil {
				log.Fatal(err)
			}
			return match
		}

		waitForChanges(command, matchFn)
	}

}

func isWhiteSpace(str string) bool {
	for _, c := range str {
		// check if the character is a whitespace character
		if !unicode.IsSpace(c) {
			return false
		}
	}

	return true
}

func gitIgnoreMatch(ignoreExpressions []string, path string) bool {
	var ignorePath bool = false
	for _, ignore := range ignoreExpressions {
		if strings.HasPrefix(path, ignore) {
			ignorePath = true
			break
		}
	}

	return !ignorePath
}

func deleteEmpty(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" && !isWhiteSpace(str) {
			r = append(r, str)
		}
	}
	return r
}

func runCommand(command string) (*exec.Cmd, error) {
	timestamp := time.Now()
	fmt.Printf("[%s] cmd = \"%s\"\n", timestamp.Format(time.UnixDate), command)
	cmdSegments := strings.Split(command, " ")
	cmd := exec.Command(cmdSegments[0], cmdSegments[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			tmp := make([]byte, 0x1000)
			_, err := stdout.Read(tmp)
			fmt.Print(string(tmp))
			if err != nil {
				break
			}
		}
	}()

	return cmd, nil
}

// Here's another comment
func waitForChanges(command string, matchFn func(path string) bool) {
	var previous DirState = checkForChanges(".", matchFn)
	var current DirState = previous

	cmd, err := runCommand(command)

	if err != nil {
		fmt.Printf("Command failed: %s", err)
	}

	for {
		time.Sleep(250 * time.Millisecond)
		current = checkForChanges(".", matchFn)

		if current.fileCount != previous.fileCount || current.hashSum != previous.hashSum {
			// If process hasn't died yet we kill it.
			if cmd.ProcessState.ExitCode() == -1 {
				pgid, err := syscall.Getpgid(cmd.Process.Pid)

				// if there are no errors, or the process doesn't exist
				// anymore, then don't exit process
				if err == nil {
					syscall.Kill(-pgid, 15)
				} else if !errors.Is(err, syscall.ESRCH) {
					log.Fatal(err)
				}
			}
			cmd, err = runCommand(command)
			if err != nil {
				fmt.Printf("Command failed: %s", err)
			}
		}

		previous = current
	}
}

func checkForChanges(cwd string, matchFn func(path string) bool) DirState {
	var paths []string

	filepath.WalkDir(cwd, func(path string, entry fs.DirEntry, err error) error {
		matched := matchFn(path)
		if matched && !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})

	hash := sha1.New()

	for _, path := range paths {
		f, err := os.Open(path)
		if err == nil {
			defer f.Close()

			if _, err := io.Copy(hash, f); err != nil {
				log.Fatal(err)
			}
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

func parseIgnore(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	text := string(data)
	expressions := strings.Split(text, "\n")
	expressions = append(expressions, ".git")

	var ignoreExpressions = deleteEmpty(expressions)

	for i, expression := range ignoreExpressions {
		if strings.HasPrefix(expression, "./") {
			ignoreExpressions[i] = expression[2:]
		}
	}

	return ignoreExpressions, nil
}
