package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

func experiment() {
	fmt.Println("Experiment! nothing works!")
	os.Exit(1)
}

func main() {
	root := filepath.Dir(os.Args[0])
	ruby := filepath.Join(root, "gitlab-shell-ruby")

	config, err := config.New()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if config.Experimental {
		experiment()
	} else {
		execErr := syscall.Exec(ruby, os.Args, os.Environ())
		if execErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to exec(%q): %v\n", ruby, execErr)
			os.Exit(1)
		}
	}
}
