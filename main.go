package main

import (
	"os"

	"github.com/angusburroughs/learnBlockChain/cli"
)

func main() {
	defer os.Exit(0)
	cli := cli.CommandLine{}
	cli.Run()
}
