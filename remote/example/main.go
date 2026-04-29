// Example shows common remote operations against the seoy lab machine.
//
// Usage:
//
//	REMOTE_PASS=<password> go run ./example
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/HeaInSeo/infra-lab/remote"
)

func main() {
	cfg := remote.DefaultConfig()
	if cfg.Password == "" {
		fmt.Fprintln(os.Stderr, "set REMOTE_PASS environment variable")
		os.Exit(1)
	}

	c, err := remote.Dial(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// 1. 도구 설치 상태 확인
	c.CheckPrereqs()

	// 2. multipass VM 목록
	fmt.Println("\n=== multipass VMs ===")
	c.MustRun("vms", "multipass list")

	// 3. K8s 노드 상태
	fmt.Println("\n=== K8s nodes ===")
	out, err := c.ClusterNodes()
	if err != nil {
		fmt.Println("nodes:", err)
	} else {
		fmt.Println(out)
	}

	// 4. Cilium 상태
	fmt.Println("\n=== Cilium status ===")
	out, err = c.CiliumStatus()
	if err != nil {
		fmt.Println("cilium:", err)
	} else {
		fmt.Println(out)
	}
}
