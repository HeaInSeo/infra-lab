package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/HeaInSeo/infra-lab/mcp/internal/server"
)

func main() {
	transport := flag.String("transport", "stdio", "MCP transport (stdio)")
	setup := flag.Bool("setup", false, "Run an interactive setup menu")
	doctor := flag.Bool("doctor", false, "Check infra-lab MCP readiness and exit")
	printClientConfig := flag.String("print-client-config", "", "Print client configuration for codex or claude and exit")
	flag.Parse()

	if *setup || (flag.NFlag() == 0 && isTerminal(os.Stdin)) {
		if err := server.RunSetupMenu(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "infra-lab-mcp setup: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *doctor {
		out, err := server.SetupCheckText()
		if err != nil {
			fmt.Fprintf(os.Stderr, "infra-lab-mcp doctor: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprint(os.Stdout, out)
		return
	}
	if *printClientConfig != "" {
		out, err := server.ClientConfigText(*printClientConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "infra-lab-mcp client config: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprint(os.Stdout, out)
		return
	}

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

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
