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
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
)

type DirState struct {
	Hash      string
	FileCount int
}

type ProgramState struct {
	Command  string
	MatchFn  func(path string) bool
	Interval time.Duration
	Files    []string
}

func main() {
	// * Place a watcher on each file.
	// * When a file is changed, we traverse the directories
	//   and figure out which files have been added and add watchers to those files.
	// * When a file is changed, if it's removed, delete the file from our set of watched files.

	var intervalMs time.Duration
	var command string
	var matchRegexp string
	var help bool

	flag.StringVar(&command, "c", "", "The shell command to run.")
	flag.StringVar(&matchRegexp, "E", "", "The match expression. If a change occurs in a file that matches this regex then the command will be run.")
	flag.BoolVar(&help, "h", false, "The help flag.")
	flag.DurationVar(&intervalMs, "n", time.Second, "The amount of time between checking the watched files for changes.")
	flag.Parse()

	files := flag.Args()

	if help {
		printHelp()
		os.Exit(0)
	}

	if command == "" {
		fmt.Fprintf(os.Stderr, "Error: command not specified!\n")
		printHelp()
		os.Exit(1)
	}

	if matchRegexp == "" {
		if len(files) > 0 {
			matchFn := func(path string) bool {
				return false
			}
			state := ProgramState{
				Command:  command,
				MatchFn:  matchFn,
				Interval: intervalMs,
				Files:    files,
			}
			waitForChanges(state)
		} else {
			ignoreExpressions, err := parseIgnore(".gitignore")
			if err != nil {
				log.Fatal(err)
			}

			matchFn := func(path string) bool {
				return gitIgnoreMatch(ignoreExpressions, path)
			}
			state := ProgramState{
				Command:  command,
				MatchFn:  matchFn,
				Interval: intervalMs,
				Files:    files,
			}
			waitForChanges(state)
		}
	} else {
		matchFn := func(path string) bool {
			match, err := regexp.MatchString(matchRegexp, path)
			if err != nil {
				log.Fatal(err)
			}
			return match
		}

		state := ProgramState{
			Command:  command,
			MatchFn:  matchFn,
			Interval: intervalMs,
			Files:    files,
		}
		waitForChanges(state)
	}

}

func printHelp() {
	fmt.Fprintf(os.Stderr, "Usage:\n  fmon [options] [files...]\n\nOPTIONS\n")
	flag.PrintDefaults()
}

// Returns `true` if the string contains only whitespace
// characters, returns false otherwise.
func isWhiteSpace(str string) bool {
	for _, c := range str {
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

	fmt.Printf("[%s] cmd = \"%s\"\n", timestamp.Format(time.DateTime), command)

	cmdSegments := strings.Split(command, " ")
	cmd := exec.Command(cmdSegments[0], cmdSegments[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stderr = cmd.Stdout

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

func waitForChanges(state ProgramState) {
	var wg sync.WaitGroup
	var current DirState = checkForChanges(".", state.MatchFn, state.Files)

	cmd, err := runCommand(state.Command)

	if err != nil {
		fmt.Printf("Command failed: %s", err)
	}

	controlSignal := make(chan os.Signal, 1)
	signal.Notify(controlSignal, os.Interrupt)
	go func() {
		_ = <-controlSignal
		err = safeKill(cmd)
		if err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for {
			time.Sleep(state.Interval)
			newState := checkForChanges(".", state.MatchFn, state.Files)

			if current.FileCount != newState.FileCount || current.Hash != newState.Hash {
				// If process hasn't died yet we kill it.
				err = safeKill(cmd)
				if err != nil {
					log.Fatal(err)
				}
				cmd, err = runCommand(state.Command)
				if err != nil {
					fmt.Printf("Command failed: %s", err)
				}
			}

			current = newState
		}
	}()

	wg.Wait()
}

func checkForChanges(cwd string, matchFn func(path string) bool, files []string) DirState {
	var paths []string
	paths = append(paths, files...)

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
		Hash:      hashString,
		FileCount: len(paths),
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

// Kills command process and all child processes.
func safeKill(cmd *exec.Cmd) error {
	if cmd.ProcessState.ExitCode() == -1 {
		pgid, err := syscall.Getpgid(cmd.Process.Pid)

		// if there are no errors, or the process doesn't exist
		// anymore, then don't exit process
		if err == nil {
			syscall.Kill(-pgid, 15)
		} else if !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}

	return nil
}
