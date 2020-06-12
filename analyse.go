package main

import (
	"flag"
	"fmt"
)

func main() {
	path := flag.String("f", "", "Path to go file")
	console := flag.Bool("c", false, "Print uml diagram to console")
	flag.Parse()

	fs := NewFileSet()
	// if fs.smachinePkg != "" {
	// 	name := `\Insolar\go\src\github.com\insolar\assured-ledger\ledger-core\virtual\execute\execute.go`
	// 	fs.AddFile(name)
	// 	fs.WriteUMLs(false)
	// 	return
	// }

	if path == nil || *path == "" {
		fmt.Print("Error: Path was not specified\n")
		return
	}

	fs.AddFile(*path)
	fs.WriteUMLs(console != nil && *console)
}
