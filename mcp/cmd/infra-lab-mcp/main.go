package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/HeaInSeo/infra-lab/mcp/internal/server"
)

func main() {
	transport := flag.String("transport", "stdio", "MCP transport (stdio)")
	flag.Parse()

	if *transport != "stdio" {
		fmt.Fprintf(os.Stderr, "unsupported transport: %s\n", *transport)
		os.Exit(2)
	}

	srv, err := server.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "infra-lab-mcp: %v\n", err)
		os.Exit(1)
	}

	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "infra-lab-mcp: %v\n", err)
		os.Exit(1)
	}
}
