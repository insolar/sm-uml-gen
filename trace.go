package main

import (
	"go/ast"
	"go/token"
	"strings"
)

var contextMarker = &StateUpdate{isContext: true}

type ExecTrace struct {
	md     *MethodDecl
	parent *ExecTrace
	fs     *File
	traced map[string]*StateUpdate
	usages map[string]struct{}

	conds    []ast.Expr
	inverted bool

	migration    ast.Expr
	errorHandler ast.Expr
	stepFlags    ast.Expr
}

func (p *ExecTrace) isTraced(n string) bool {
	switch {
	case p.traced != nil:
		return p.traced[n] != nil
	case p.parent != nil:
		return p.parent.isTraced(n)
	default:
		return p.md.CtxArg == n
	}
}

func (p *ExecTrace) getTraced(n string) *StateUpdate {
	switch {
	case p.traced != nil:
		return p.traced[n]
	case p.parent != nil:
		return p.parent.getTraced(n)
	case p.md.CtxArg == n:
		return contextMarker
	default:
		return nil
	}
}

func (p *ExecTrace) _copyInherited() map[string]*StateUpdate {
	switch {
	case p.traced != nil:
		cp := make(map[string]*StateUpdate)
		for k, v := range p.traced {
			if v != nil {
				cp[k] = v
			}
		}
		return cp
	case p.parent != nil:
		return p.parent._copyInherited()
	case p.md.CtxArg != "":
		return map[string]*StateUpdate{p.md.CtxArg: contextMarker}
	default:
		return map[string]*StateUpdate{}
	}
}

func (p *ExecTrace) setTraced(n string, upd *StateUpdate) {
	if p.traced == nil {
		if upd == nil && !p.isTraced(n) {
			return
		}
		p.traced = p._copyInherited()
	}

	if upd == nil {
		delete(p.traced, n)
	} else {
		p.traced[n] = upd
	}
}

func (p *ExecTrace) remapContextNames(lhs []string, rhs []exprResult) {
	switch {
	case len(lhs) == 0:
		return
	case len(rhs) > 0:
		hasRhs := false
		for i, n := range lhs {
			switch {
			case n == "":
				continue
			case i >= len(rhs):
				// multi-arg return from func
				break
			case rhs[i].name != nil:
				if traced := p.getTraced(*rhs[i].name); traced != nil {
					hasRhs = true
					rhs[i].upd = traced
				}
			}
		}
		if !hasRhs {
			break
		}

		for i, n := range lhs {
			switch {
			case n == "":
				continue
			case len(rhs) == 1:
				// multi-arg return from func
				p.setTraced(n, rhs[0].upd)
			default:
				p.setTraced(n, rhs[i].upd)
			}
		}
		return
	}

	for _, n := range lhs {
		if n != "" {
			p.setTraced(n, nil)
		}
	}
}

func (p *ExecTrace) spawn() *ExecTrace {
	return &ExecTrace{md: p.md, parent: p, fs: p.fs, migration: p.migration, errorHandler: p.errorHandler, stepFlags: p.stepFlags}
}

func (p *ExecTrace) spawnCase(conds []ast.Expr) *ExecTrace {
	et := p.spawn()
	et.conds = conds
	return et
}

func (p *ExecTrace) spawnIf(cond ast.Expr, inverted bool) *ExecTrace {
	et := p.spawn()
	et.conds = []ast.Expr{cond}
	et.inverted = inverted
	return et
}

func (p *ExecTrace) nearestCond() *ExecTrace {
	if len(p.conds) > 0 {
		return p
	}
	if p.parent != nil {
		return p.parent.nearestCond()
	}
	return nil
}

func (p *ExecTrace) parseBlockStmt(stmt *ast.BlockStmt) {
	if stmt == nil {
		return
	}
	p.parseStatements(stmt.List)
}

func (p *ExecTrace) parseStatements(list []ast.Stmt) {
	if len(list) == 0 {
		return
	}

	et := p.spawn()
	su := et._parseStatements(list)

	p.collectUsages(et)
	p.migration, p.errorHandler, p.stepFlags = et.migration, et.errorHandler, et.stepFlags

	if su != nil {
		p.addTransition(su)
	}
}

func (p *ExecTrace) collectUsages(from *ExecTrace) {
	for k := range from.usages {
		if p.usages == nil {
			p.usages = make(map[string]struct{})
		}
		p.usages[k] = struct{}{}
	}
}

func (p *ExecTrace) _parseDecl(decl *ast.GenDecl) {
	switch decl.Tok {
	case token.CONST:
		for _, spec := range decl.Specs {
			vSpec := spec.(*ast.ValueSpec)
			for _, n := range vSpec.Names {
				p.setTraced(n.Name, nil)
			}
		}
	case token.VAR:
		for _, spec := range decl.Specs {
			vSpec := spec.(*ast.ValueSpec)
			p.remapContextNames(p.identToNames(vSpec.Names), p.exprToValues(vSpec.Values))
		}
	case token.TYPE:
		for _, spec := range decl.Specs {
			vSpec := spec.(*ast.TypeSpec)
			if vSpec.Name != nil {
				p.setTraced(vSpec.Name.Name, nil)
			}
		}
	}
}

func (p *ExecTrace) _parseStatements(list []ast.Stmt) *StateUpdate {
	for _, stmt := range list {
		for {
			switch op := stmt.(type) {
			case *ast.DeclStmt:
				switch decl := op.Decl.(type) {
				case *ast.GenDecl:
					p._parseDecl(decl)
				case *ast.FuncDecl:
					// skip
				}
			case *ast.LabeledStmt:
				stmt = op.Stmt
				continue
			case *ast.ExprStmt:
				p.parseCallToCtx(op.X)
			case *ast.AssignStmt:
				p.remapContextNames(p.exprToNames(op.Lhs), p.exprToValues(op.Rhs))
			case *ast.ReturnStmt:
				switch {
				case len(op.Results) == 0:
					// named return params
					// TODO not implemented
				case p.md.UpdateIdx == 0:
					// this is a non-context func
					return p.exprToResult(op.Results[0])
				case p.md.UpdateIdx > len(op.Results):
				default:
					return p.exprToResult(op.Results[p.md.UpdateIdx-1])
				}
				return nil

			case *ast.BranchStmt:
				// BREAK, CONTINUE, GOTO, FALLTHROUGH
				// These will be handled by other traces, yet we can loose info about setting
				// TODO warn when specific context settings are present
				return nil
			case *ast.BlockStmt:
				p.parseStatements(op.List)
			case *ast.IfStmt:
				if op.Body != nil && len(op.Body.List) > 0 {
					p.spawnIf(op.Cond, false).parseStatements(op.Body.List)
				}
				if op.Else != nil {
					p.spawnIf(op.Cond, true).parseStatements([]ast.Stmt{op.Else})
				}
			case *ast.CaseClause:
				if len(op.Body) > 0 {
					p.spawnCase(op.List).parseStatements(op.Body)
				}
			case *ast.SwitchStmt:
				p.parseBlockStmt(op.Body)
			case *ast.TypeSwitchStmt:
				p.parseBlockStmt(op.Body)
			case *ast.CommClause:
				body := op.Body
				switch {
				case op.Comm == nil:
				case len(body) == 0:
					body = []ast.Stmt{op.Comm}
				default:
					body = make([]ast.Stmt, 1, len(op.Body)+1)
					body[0] = op.Comm
					body = append(body, op.Body...)
				}
				p.parseStatements(body)
			case *ast.SelectStmt:
				p.parseBlockStmt(op.Body)
			case *ast.ForStmt:
				p.parseBlockStmt(op.Body)
			case *ast.RangeStmt:
				p.parseBlockStmt(op.Body)
			case *ast.DeferStmt:
				// op.Call
				// TODO defer
			}
			break
		}
	}
	return nil
}

type exprResult struct {
	name *string
	upd  *StateUpdate
}

func (p *ExecTrace) exprToNames(exprs []ast.Expr) []string {
	if len(exprs) == 0 {
		return nil
	}

	s := make([]string, len(exprs))
	count := 0
	for i, ex := range exprs {
		switch x, sel := getSelectorOfExpr(ex); {
		case x != "":
		case sel == "":
		default:
			count++
			s[i] = sel
		}
	}

	if count == 0 {
		return nil
	}
	return s
}

func (p *ExecTrace) exprToResult(expr ast.Expr) *StateUpdate {
	switch mt := p.md.MType; {
	case mt.HasStateUpdate():
	case mt.HasContextArg():
		if op, ok := expr.(*ast.UnaryExpr); ok {
			if op.Op != token.AND {
				break
			}
			expr = op.X
		}
		if op, ok := expr.(*ast.CompositeLit); ok {
			sel := p.getInlineFuncExpr(op, 0)
			if sel != "" {
				return newStateUpdate(nil, sel)
			}
		}
	}
	return p.exprToValue(expr)
}

func (p *ExecTrace) exprToValue(expr ast.Expr) *StateUpdate {
	switch arg := expr.(type) {
	case *ast.SelectorExpr:
		sel := ""
		if arg.Sel != nil {
			sel = arg.Sel.Name
		}
		if parent := p.exprToValue(arg.X); parent != nil {
			return newStateUpdate(parent, sel)
		}
		return nil
	case *ast.CallExpr:
		call := p.exprToValue(arg.Fun)
		call.isCall = true
		// TODO check for context in args
		if call != nil {
			call.args = arg.Args
		}
		return call
	case *ast.Ident:
		su := p.getTraced(arg.Name)
		if su != nil {
			return su
		}
		return &StateUpdate{name: arg.Name}
	}
	return nil
}

func (p *ExecTrace) parseCallToCtx(expr ast.Expr) {
	switch arg := expr.(type) {
	case *ast.CallExpr:
		call := p.exprToValue(arg.Fun)
		switch {
		case call == nil:
			return
		case call.parent != contextMarker:
			p.lookForAdapterCall(call)
			return
		case len(arg.Args) != 1:
			return
		}

		switch call.name {
		case "SetDefaultMigration":
			p.migration = arg.Args[0]
		case "SetDefaultFlags":
			p.stepFlags = arg.Args[0]
		case "SetDefaultErrorHandler":
			p.errorHandler = arg.Args[0]
		}
		if p.usages == nil {
			p.usages = make(map[string]struct{})
		}
		p.usages[call.name] = struct{}{}
	}
}

func (p *ExecTrace) lookForAdapterCall(su *StateUpdate) {
	if len(su.args) != 0 {
		return
	}

	switch su.name {
	case "Start":
	case "Send":
	default:
		return
	}

	top := su

	for su = su.parent; su.HasName(); su = su.parent {
		if strings.HasPrefix(su.name, "Prepare") {
			if p.hasContextArg(su.args) >= 0 {
				p.addAdapterCall(top, su)
			}
			return
		}
	}
}

func (p *ExecTrace) hasContextArg(args []ast.Expr) int {
	for i, arg := range args {
		if p.isContextArg(arg) {
			return i
		}
	}
	return -1
}

func (p *ExecTrace) isContextArg(arg ast.Expr) bool {
	return p.exprToValue(arg) == contextMarker
}

func (p *ExecTrace) exprToValues(exprs []ast.Expr) []exprResult {
	if len(exprs) == 0 {
		return nil
	}
	s := make([]exprResult, len(exprs))
	count := 0
	for i, expr := range exprs {
		su := p.exprToValue(expr)
		if su != nil {
			count++
			s[i].upd = su
			continue
		}

		switch x, sel := getSelectorOfExpr(expr); {
		case x != "":
		case sel == "":
		default:
			count++
			s[i].name = &sel
		}
	}

	if count == 0 {
		return nil
	}
	return s
}

func (p *ExecTrace) identToNames(names []*ast.Ident) []string {
	if len(names) == 0 {
		return nil
	}
	s := make([]string, len(names))
	for i, n := range names {
		s[i] = n.Name
	}
	return s
}
