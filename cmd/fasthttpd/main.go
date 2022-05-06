package main

import (
	"log"
	"os"

	"github.com/fasthttpd/fasthttpd/pkg/cmd"
)

func main() {
	if err := cmd.RunFastHttpd(os.Args); err != nil {
		log.Fatal(err)
	}
}
