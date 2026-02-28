package main

import (
	"bytes"
	"fmt"
	"os"

	lua "github.com/tos-network/golua"
	"github.com/tos-network/golua/parse"
)

func stripShebang(src []byte) []byte {
	if len(src) == 0 || src[0] != '#' {
		return src
	}
	if i := bytes.IndexByte(src, '\n'); i >= 0 {
		return src[i+1:]
	}
	return []byte{}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: lua54_compilecheck <lua-file>")
		os.Exit(2)
	}
	path := os.Args[1]
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	b = stripShebang(b)

	chunk, err := parse.Parse(bytes.NewReader(b), path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := lua.Compile(chunk, path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
