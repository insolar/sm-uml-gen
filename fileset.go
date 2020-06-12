package main

import (
	"bufio"
	"fmt"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func NewFileSet() *FileSet {
	return &FileSet{
		fs:           token.NewFileSet(),
		umlExtension: ".plantuml",
		smachinePkg:  `github.com/insolar/assured-ledger/ledger-core/conveyor/smachine`,
	}
}

type FileSet struct {
	fs    *token.FileSet
	files map[token.Pos]*File

	umlExtension string
	smachinePkg  string

	types map[string]*SMDecl
}

func (p *FileSet) AddFile(filename string) {
	src, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println("Failed to read file:", filename)
		panic(err)
	}

	base := p.fs.Base()
	fileAst, err := parser.ParseFile(p.fs, filename, src, parser.ParseComments)
	if err != nil {
		fmt.Println("Failed to parse file:", filename)
		panic(err)
	}

	output := filename
	if ext := filepath.Ext(output); ext != "" {
		output = output[:len(output)-len(ext)]
	}
	fileInfo := File{fs: p, output: output, src: src, base: token.Pos(base)}
	fileInfo.parseAst(fileAst)
	fileInfo.src = nil
}

func (p *FileSet) AddStep(output string, md *MethodDecl) {
	if p.types == nil {
		p.types = map[string]*SMDecl{}
	}
	rt := p.types[md.RType]
	if rt == nil {
		rt = &SMDecl{RType: md.RType, Output: output, SeqNo: len(p.types)}
		p.types[rt.RType] = rt
	}
	rt.AddStep(md, false)
}

func (p *FileSet) WriteUMLs(console bool) {
	if len(p.types) == 0 {
		return
	}
	if console {
		p.writePagedUML("-")
		return
	}
	p.writePagedUML("")
}

func (p *FileSet) writePagedUML(singleFile string) {
	decls := make([]*SMDecl, 0, len(p.types))
	for _, d := range p.types {
		decls = append(decls, d)
	}
	sort.SliceStable(decls, func(i, j int) bool {
		if n := strings.Compare(decls[i].Output, decls[j].Output); n != 0 {
			return n < 0
		}
		return strings.Compare(decls[i].RType, decls[j].RType) < 0
	})

	if singleFile != "" {
		p.writeUML(singleFile, decls)
		return
	}

	lastPos := 0
	lastOut := decls[0].Output
	for i := 1; i < len(decls); i++ {
		if decls[i].Output == lastOut {
			continue
		}
		p.writeUML(lastOut+p.umlExtension, decls[lastPos:i])
		lastOut = decls[i].Output
		lastPos = i
	}
	p.writeUML(lastOut+p.umlExtension, decls[lastPos:])
}

func (p *FileSet) writeManyUML() {
	for _, d := range p.types {
		output := d.Output + `_` + d.RType + p.umlExtension
		p.writeUML(output, []*SMDecl{d})
	}
}

func (p *FileSet) writeUML(output string, decls []*SMDecl) {
	var file *os.File
	if output == "-" {
		file = os.Stdout
	} else {
		var err error
		file, err = os.Create(output)
		if err != nil {
			fmt.Println("Failed to create file:", output)
			panic(err)
		}
		defer func() {
			_ = file.Close()
		}()
	}

	out := bufio.NewWriter(file)
	defer func() {
		_ = out.Flush()
	}()

	w := Writer{out: out, output: output}
	w.L(`@startuml`)
	for _, d := range decls {
		d.Propagate()
	}
	for _, d := range decls {
		// if i > 0 {
		// 	w.L(`newpage`)
		// }
		w.WriteDecl(d)
	}

	w.L(`@enduml`)
}
