package main

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

type File struct {
	fs     *FileSet
	output string

	base token.Pos
	src  []byte

	smachinePkg string
}

func (p *File) parseAst(fileAst *ast.File) {
	for _, imp := range fileAst.Imports {
		switch pkg, err := strconv.Unquote(imp.Path.Value); {
		case err != nil:
			panic(err)
		case pkg != p.fs.smachinePkg:
			continue
		case imp.Name != nil:
			p.smachinePkg = imp.Name.Name
		default:
			p.smachinePkg = p.fs.smachinePkg
			if n := strings.LastIndexByte(p.smachinePkg, '/'); n >= 0 {
				p.smachinePkg = p.smachinePkg[n+1:]
			}
		}
		break
	}

	if p.smachinePkg == "" {
		return
	}

	for _, decl := range fileAst.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			md := p.parseFuncDecl(fd)
			if md == nil {
				continue
			}

			md.parseFuncBody(fd.Body, p)
			p.fs.AddStep(p.output, md)
		}
	}
}

const TypeStateUpdate = "StateUpdate"
const GetInitStateForFunc = "GetInitStateFor"
const GetSubroutineInitState = "GetSubroutineInitState"

func (p *File) parseFuncDecl(fd *ast.FuncDecl) *MethodDecl {
	if fd.Type.Results == nil {
		return nil
	}

	md := p.findStateUpdate(fd.Type.Results.List)

	checkContext := false
	switch {
	case md != nil:
		checkContext = true
		if fd.Name != nil {
			md.Name = fd.Name.Name
		}
	case fd.Name == nil:
		return nil
	default:
		switch fd.Name.Name {
		case GetInitStateForFunc, GetSubroutineInitState:
			md = p.findFuncWith(fd.Type.Results.List, fd.Name.Name, "InitFunc")
		}
		if md == nil {
			return nil
		}
	}

	if fd.Recv != nil {
		if len(fd.Recv.List) != 1 {
			return nil
		}
		id := fd.Recv.List[0]
		switch len(id.Names) {
		case 0:
		case 1:
			md.RName = id.Names[0].Name
		default:
			// wtf?
			return nil
		}

		switch x, sel, _ := getTypeOfExpr(id.Type); {
		case sel == "":
			return nil
		case x != "":
			return nil
		default:
			md.RType = sel
		}
	}

	if checkContext && fd.Type.Params != nil {
		md.MType, md.CtxArg = p.findContextArg(fd.Type.Params.List)
	}

	return md
}

func (p *File) findStateUpdate(retFields []*ast.Field) *MethodDecl {
	md := MethodDecl{}

	md.UpdateIdx, md.UpdateArg = p.findResultArg(retFields, TypeStateUpdate, false)
	if md.UpdateIdx == 0 {
		return nil
	}

	return &md
}

func (p *File) findFuncWith(retFields []*ast.Field, funcName, resTypeName string) *MethodDecl {
	md := MethodDecl{}

	if idx, _ := p.findResultArg(retFields, resTypeName, true); idx == 0 {
		return nil
	}

	md.Name = funcName
	md.MType = DeclarationInit
	return &md
}

func (p *File) findResultArg(retFields []*ast.Field, typeName string, onlyOne bool) (retPos int, retName string) {
	argPos := 0
	for _, retArg := range retFields {
		switch n := len(retArg.Names); n {
		case 0, 1:
			//
		default:
			if onlyOne {
				return 0, ""
			}
			argPos += n
			continue
		}
		if onlyOne && argPos > 0 {
			return 0, ""
		}
		argPos++

		switch x, sel := getSelectorOfExpr(retArg.Type); {
		case sel != typeName:
			continue
		case x != p.smachinePkg:
			continue
		}

		if retPos > 0 {
			return 0, ""
		}
		retPos = argPos
		if len(retArg.Names) > 0 {
			retName = retArg.Names[0].Name
		}
	}

	return
}

func (p *File) findContextArg(params []*ast.Field) (MethodType, string) {
	for _, inArg := range params {
		x, sel := getSelectorOfExpr(inArg.Type)
		if x != p.smachinePkg {
			continue
		}

		argName := ""
		if len(inArg.Names) > 0 {
			argName = inArg.Names[0].Name
		}

		switch sel {
		case "InitializationContext":
			return Initialization, argName
		case "ExecutionContext":
			return Execution, argName
		case "MigrationContext":
			return Migration, argName
		case "ConstructionContext":
			return Construction, argName
		}
	}

	return 0, ""
}

func (p *File) Excerpt(pos token.Pos, end token.Pos, maxLen int) string {
	pos -= p.base
	end -= p.base
	if int(end-pos) > maxLen {
		return string(p.src[pos:pos+maxCondLen]) + `...`
	}
	return string(p.src[pos:end])
}

func getTypeOfExpr(expr ast.Expr) (x, sel string, star bool) {
	switch arg := expr.(type) {
	case *ast.StarExpr:
		star = true
		expr = arg.X
	case *ast.SelectorExpr:
	default:
		return "", "", false
	}
	x, sel = getSelectorOfExpr(expr)
	return
}

func getSelectorOfExpr(expr ast.Expr) (x, sel string) {
	switch arg := expr.(type) {
	case *ast.SelectorExpr:
		if arg.Sel != nil {
			sel = arg.Sel.Name
		}
		if id, ok := arg.X.(*ast.Ident); ok {
			x = id.Name
		}
	case *ast.Ident:
		return "", arg.Name
	}
	return
}
