package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/awaleha/stygai-status/internal/zfs"
)

func main() {
	fmt.Println("stygai-status v0.1")

	ctx := context.Background()
	pool, err := zfs.Status(ctx)
	if err != nil {
		fmt.Println("Error zfs.go: ", err)
	}

	jpool, err := json.MarshalIndent(pool, "", " ")
	if err != nil {
		fmt.Println("Error json Marshal: ", err)
	}
	fmt.Println(string(jpool))
}
