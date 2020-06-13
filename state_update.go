package main

import (
	"go/ast"
)

func newStateUpdate(parent *StateUpdate, name string) *StateUpdate {
	return &StateUpdate{parent: parent, name: name, isContext: parent != nil && parent.isContext}
}

type StateUpdate struct {
	parent    *StateUpdate
	name      string
	args      []ast.Expr
	isContext bool
	isCall    bool
}

func (u StateUpdate) fullName() string {
	if u.parent == nil {
		return u.name
	}
	return u.parent.fullName() + `.` + u.name
}

func (u *StateUpdate) HasName() bool {
	return u != nil && u.name != ""
}
