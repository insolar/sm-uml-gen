package main

import (
	"go/ast"
	"strings"
)

const maxCondLen = 30

type MethodType uint8

func (t MethodType) HasStateUpdate() bool {
	return t >= Initialization
}

func (t MethodType) HasContextArg() bool {
	return t >= Construction
}

const (
	_ MethodType = iota
	DeclarationInit
	Construction
	Initialization
	Execution
	Migration
)

type MethodDecl struct {
	SM     *SMDecl
	RType  string
	RName  string
	Name   string
	CtxArg string
	MType  MethodType

	UpdateArg string
	UpdateIdx int

	Transitions []MethodTransition
	SubSteps    []*MethodDecl
	RepeatTrIdx int

	Usages       map[string]struct{}
	Migrations   map[string]struct{}
	StepNo       int
	Duplicate    bool
	IsSubroutine bool
	CanPropagate bool
}

type MethodTransition struct {
	Condition        string
	Operation        string
	Transition       string
	Migration        string
	InheritMigration bool

	HiddenPropagate string
	TransitionTo    *MethodDecl
	HiddenPropTo    *MethodDecl
	MigrationTo     *MethodDecl
}

func (p *MethodDecl) parseFuncBody(bodyAst *ast.BlockStmt, fs *File) {
	if bodyAst == nil {
		return
	}

	et := ExecTrace{md: p, fs: fs}
	et.parseStatements(bodyAst.List)

	p.Usages = et.usages
}

func (p *MethodDecl) IsEmpty() bool {
	return len(p.Migrations) > 0
}

func (p *MethodDecl) AddMigration(migration string) bool {
	if p.Migrations == nil {
		p.Migrations = map[string]struct{}{migration: {}}
		return true
	}
	if _, ok := p.Migrations[migration]; !ok {
		p.Migrations[migration] = struct{}{}
		return true
	}
	return false
}

func (p *MethodDecl) AddMigrations(step *MethodDecl) bool {
	result := false
	for k := range step.Migrations {
		if p.AddMigration(k) {
			result = true
		}
	}
	return result
}

func (p *MethodDecl) GetRepeatTransitionIdx() int {
	switch {
	case p.RepeatTrIdx > 0:
		return p.RepeatTrIdx
	case len(p.Transitions) == 0:
		return -1
	case p.Transitions[0].Transition == "":
		return 0
	default:
		return -1
	}
}

func (p *MethodDecl) AddTransition(tr MethodTransition) {
	if tr.Transition != "" {
		p.Transitions = append(p.Transitions, tr)
		return
	}

	if tr.Migration != "" {
		p.AddMigration(tr.Migration)
	}

	repeatIdx := p.GetRepeatTransitionIdx()
	if repeatIdx < 0 {
		p.RepeatTrIdx = len(p.Transitions)
		p.Transitions = append(p.Transitions, tr)
		return
	}

	repTr := &p.Transitions[repeatIdx]

	switch {
	case tr.Condition == "":
	case repTr.Condition == "":
		repTr.Condition = tr.Condition
	case strings.HasPrefix(repTr.Condition, "..."):
	default:
		repTr.Condition += `\n` + tr.Condition
		if len(repTr.Condition) >= maxCondLen {
			repTr.Condition += `...`
		}
	}

	switch {
	case tr.Operation == "":
	case repTr.Operation == "":
		repTr.Operation = tr.Operation
	case strings.Contains(repTr.Operation, tr.Operation):
	default:
		repTr.Operation += `, ` + tr.Operation
	}
}
