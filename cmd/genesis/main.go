// 生成创世 JSON。用法：go run ./cmd/genesis [输出路径]
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"pole-core/contracts"
)

func main() {
	g := contracts.DefaultGenesisConfig()
	out := "config/genesis.json"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	dir := filepath.Dir(out)
	if dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	if err := g.SaveGenesisConfig(out); err != nil {
		log.Fatalf("SaveGenesisConfig: %v", err)
	}
	fmt.Printf("Genesis written to %s (chain_id=%s)\n", out, g.ChainID)
}
