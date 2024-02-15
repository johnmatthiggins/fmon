# File Monitor

A command that watches the current directory for changes and runs a shell command
when changes are detected.

```
Usage:
  fmon [options] [files...]

OPTIONS
  -E string
    	The match expression. If a change occurs in a file that matches this regex then the command will be run.
  -c string
    	The shell command to run.
  -h	The help flag.
  -n duration
    	The amount of time between checking the watched files for changes. (default 1s)
```

# TODO
- [x] Figure out how to close all child processes when SIGTERM is received.
- [x] Add --help flag.
- [ ] Refine .gitignore functionality.
- [x] Kill and rerun long running processes.
- [ ] Move from polling interval architecture to asynchronous event monitoring architecture.
