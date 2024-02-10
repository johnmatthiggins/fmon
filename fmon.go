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
	hashSum   string
	fileCount int
}

func main() {
	var intervalMs time.Duration
	var command string
	var matchRegexp string
	var help bool

	flag.StringVar(&command, "c", "ls", "The shell command to run.")
	flag.StringVar(&matchRegexp, "E", "", "The match expression. If a change occurs in a file that matches this regex then the command will be run.")
	flag.BoolVar(&help, "h", false, "The flag that pulls up the manual.")
	flag.DurationVar(&intervalMs, "n", time.Second, "The amount of time in milliseconds between checks.")
	flag.Parse()

	if help {
		printHelp()
		os.Exit(0)
	}

	var useIgnoreFile bool = matchRegexp == ""

	if useIgnoreFile {
		ignoreExpressions, err := parseIgnore(".gitignore")
		if err != nil {
			log.Fatal(err)
		}

		matchFn := func(path string) bool {
			return gitIgnoreMatch(ignoreExpressions, path)
		}
		waitForChanges(command, matchFn, intervalMs)
	} else {
		matchFn := func(path string) bool {
			match, err := regexp.MatchString(matchRegexp, path)
			if err != nil {
				log.Fatal(err)
			}
			return match
		}

		waitForChanges(command, matchFn, intervalMs)
	}

}

func printHelp() {
	fmt.Printf("Usage:\n  fmon [options]\n\nOPTIONS\n")
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

func waitForChanges(command string, matchFn func(path string) bool, interval time.Duration) {
	var wg sync.WaitGroup
	var current DirState = checkForChanges(".", matchFn)

	cmd, err := runCommand(command)

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
			time.Sleep(interval * time.Millisecond)
			newState := checkForChanges(".", matchFn)

			if current.fileCount != newState.fileCount || current.hashSum != newState.hashSum {
				// If process hasn't died yet we kill it.
				err = safeKill(cmd)
				if err != nil {
					log.Fatal(err)
				}
				cmd, err = runCommand(command)
				if err != nil {
					fmt.Printf("Command failed: %s", err)
				}
			}

			current = newState
		}
	}()

	wg.Wait()
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
