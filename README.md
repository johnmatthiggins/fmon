# File Monitor

A command that watches the current directory for changes and runs a shell command
when changes are detected.

# TODO
- [x] Figure out how to close all child processes when SIGTERM is received.
- [ ] Add --help flag.
- [ ] Refine .gitignore functionality.
- [x] Kill and rerun long running processes.
- [ ] Move from polling intetrval architecture to asynchronous event monitoring architecture.
