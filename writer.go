package main

import (
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Writer struct {
	output    string
	out       *bufio.Writer
	unknownId int
}

func (p *Writer) h(_ int, err error) {
	if err != nil {
		fmt.Println("Failed to write to file:", p.output)
		fmt.Println(err.Error())
		panic(nil)
	}
}

func (p *Writer) L(s ...string) {
	p.P(s...)
	p.h(0, p.out.WriteByte('\n'))
}

func (p *Writer) P(s ...string) {
	for _, si := range s {
		p.h(p.out.WriteString(si))
	}
}

func (p *Writer) WriteDecl(d *SMDecl) {
	stepNames := make([]string, 0, len(d.Steps))
	for k := range d.Steps {
		stepNames = append(stepNames, k)
	}
	sort.Strings(stepNames)

	startType := Initialization
	if d.HasDeclInit {
		startType = DeclarationInit
	}

	for _, k := range stepNames {
		step := d.Steps[k]
		stepAlias := p.stepAlias(d, step.Name, step)

		stereotype := ""
		if step.IsSubroutine {
			stereotype = " <<sdlreceive>>"
		}

		p.L("state ", strconv.Quote(step.Name), " as ", stepAlias, stereotype)
		if !step.IsSubroutine {
			p.L(stepAlias, " : ", d.RType)
		}
		if step.Duplicate {
			p.L(stepAlias, " : ", "DUPLICATE")
		}

		// if len(step.Usages) > 0 {
		//
		// }

		if step.MType == startType {
			p.L("[*] --> ", stepAlias)
		}

		connIdx := 0

		if n := len(step.Migrations); step.MType == Execution && n > 0 {

			mirgateNames := make([]string, 0, n)
			for k := range step.Migrations {
				if k != "" {
					mirgateNames = append(mirgateNames, k)
				}
			}
			sort.Strings(mirgateNames)
			for _, k := range mirgateNames {
				toStep := p.stepAlias(d, k, d.findStep(k))
				p.jumpMigrate(stepAlias, toStep)
			}
		}

		for _, tr := range step.Transitions {
			connIdx++

			waitOperation := tr.WaitTransition

			if tr.TransitionTo == nil || !tr.TransitionTo.IsSubroutine {
				m := ""
				switch {
				case tr.Transition == "<stop>":
					//
				case tr.Migration != "":
					m = `Migrate: ` + tr.Migration
				case !tr.InheritMigration && !step.IsSubroutine:
					m = "Migrate: <nil>"
				}
				switch {
				case m == "":
				case tr.Operation == "":
					tr.Operation = m
				default:
					tr.Operation = m + `\n` + tr.Operation
				}
			}

			note := ""
			switch {
			case tr.Operation == "":
				note = tr.Condition
			case tr.Condition == "":
				note = tr.Operation
			default:
				note = tr.Condition + `\n` + tr.Operation
			}

			switch {
			case tr.Transition == "<stop>":
				p.jumpFixed(stepAlias, `[*]`, note)
				continue
			case tr.Transition == "": // self loop
				if tr.DelayedStart == "" {
					p.jump(stepAlias, stepAlias, note, waitOperation)
					continue
				}
				fork, op := p.jumpFork(d, stepAlias, tr.DelayedStart, tr.Condition, tr.Operation)
				p.jump(fork, stepAlias, op, waitOperation)

			case tr.DelayedStart != "":
				fork, op := p.jumpFork(d, stepAlias, tr.DelayedStart, tr.Condition, tr.Operation)
				toStep := p.stepAlias(d, tr.Transition, tr.TransitionTo)
				p.jump(fork, toStep, op, waitOperation)

			default:
				toStep := p.stepAlias(d, tr.Transition, tr.TransitionTo)
				p.jump(stepAlias, toStep, note, waitOperation)
			}
		}
	}
}

func (p *Writer) jumpFork(d *SMDecl, from, toAdapter, cond, op string) (forkAlias, nextOp string) {
	fork := p.newNamelessStep(d, "", " <<fork>>")
	adapter := p.stepAlias(d, toAdapter, d.findStep(toAdapter))
	p.jumpFixed(from, fork, cond)

	i := strings.IndexByte(op, '.') // MUST be there

	p.jumpFixed(fork, adapter, op[:i])
	return fork, op[i+1:]
}

func (p *Writer) jumpMigrate(from, to string) {
	p.writeConn(from, to, "--[dotted]>", "")
}

func (p *Writer) jumpFixed(from, to, note string) {
	p.writeConn(from, to, "-->", note)
}

func (p *Writer) jumpCond(from, to, note string) {
	p.writeConn(from, to, "--[dashed]>", note)
}

func (p *Writer) jump(from, to, note string, conditional bool) {
	if conditional {
		p.jumpCond(from, to, note)
	} else {
		p.jumpFixed(from, to, note)
	}
}

func (p *Writer) stepAlias(d *SMDecl, name string, step *MethodDecl) string {
	if step != nil {
		return fmt.Sprintf("T%02d_S%03d", d.SeqNo, step.StepNo)
	}

	stepAlias := p.newNamelessStep(d, name, "")
	p.L(stepAlias, " : ", d.RType)
	p.L(stepAlias, " : UNKNOWN ")
	return stepAlias
}

func (p *Writer) newNamelessStep(d *SMDecl, name, stereotype string) string {
	p.unknownId++
	stepAlias := fmt.Sprintf("T%02d_U%03d", d.SeqNo, p.unknownId)
	as := ""
	switch {
	case name != "":
		name = strconv.Quote(name)
		as = " as "
	case stereotype == "":
		return stepAlias
	}
	p.L("state ", name, as, stepAlias, stereotype)
	return stepAlias
}

func (p *Writer) writeConn(fromStep, toStep string, line, note string) {
	if note == "" {
		p.L(fromStep, " ", line, " ", toStep)
	} else {
		p.L(fromStep, " ", line, " ", toStep, " : ", note)
	}
}
