package main

import (
	"strings"
)

type SMDecl struct {
	RType       string
	Output      string
	SeqNo       int
	Steps       map[string]*MethodDecl
	HasDeclInit bool
}

func (p *SMDecl) AddStep(step *MethodDecl, addUntyped bool) {
	if p.Steps == nil {
		p.Steps = map[string]*MethodDecl{}
	}

	if !addUntyped && step.MType == 0 {
		return
	}

	if dup := p.findStep(step.Name); dup != nil {
		dup.Duplicate = true
		return
	}

	step.StepNo = 1 + len(p.Steps)

	if step.MType == DeclarationInit {
		p.HasDeclInit = true
	}

	p.Steps[step.Name] = step

	for i := range step.SubSteps {
		p.AddStep(step.SubSteps[i], true)
	}
}

func (p *SMDecl) findStep(name string) *MethodDecl {
	if name == "" {
		return nil
	}
	for {
		if step := p.Steps[name]; step != nil {
			return step
		}
		n := strings.IndexByte(name, '.')
		if n < 0 {
			return nil
		}
		name = name[n+1:]
	}
}

func (p *SMDecl) Propagate() {
	for _, step := range p.Steps {
		step.CanPropagate = false
	}

	for _, step := range p.Steps {
		for i := range step.Transitions {
			tr := &step.Transitions[i]
			tr.TransitionTo = p.findStep(tr.Transition)
			tr.HiddenPropTo = p.findStep(tr.HiddenPropagate)

			if tr.Migration == "" {
				continue
			}

			if tr.TransitionTo != nil && tr.TransitionTo.AddMigration(tr.Migration) {
				tr.TransitionTo.CanPropagate = true
			}
			if tr.HiddenPropTo != nil && tr.HiddenPropTo.AddMigration(tr.Migration) {
				tr.HiddenPropTo.CanPropagate = true
			}

			if migrationTo := p.findStep(tr.Migration); migrationTo != nil && migrationTo.AddMigration(tr.Migration) {
				migrationTo.CanPropagate = true
			}
		}
	}

	for {
		didSomething := false
		for _, step := range p.Steps {
			if !step.CanPropagate {
				continue
			}
			step.CanPropagate = false
			for _, tr := range step.Transitions {
				switch {
				case tr.TransitionTo == nil:
				case !tr.InheritMigration:
				case tr.TransitionTo.AddMigrations(step):
					tr.TransitionTo.CanPropagate = true
					didSomething = true
				}

				switch {
				case tr.HiddenPropTo == nil:
				case tr.HiddenPropTo.AddMigrations(step):
					tr.HiddenPropTo.CanPropagate = true
					didSomething = true
				}
			}
		}
		if !didSomething {
			return
		}
	}
}
