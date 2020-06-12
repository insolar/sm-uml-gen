package main

import (
	"bufio"
	"fmt"
	"sort"
	"strconv"
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
				p.writeConn(stepAlias, toStep, "--[dotted]>", "")
			}
		}

		for _, tr := range step.Transitions {
			connIdx++
			note := ""
			if tr.Operation != "" || tr.Condition != "" {
				if tr.Condition == "" {
					note = tr.Operation
				} else {
					note = tr.Condition + `\n` + tr.Operation
				}
			}

			switch tr.Transition {
			case "": // self loop
				p.writeConn(stepAlias, stepAlias, "->", note)
			case "<stop>":
				if note != "" {
					p.L(stepAlias, " -->[*] : ", note)
				} else {
					p.L(stepAlias, " -->[*]")
				}
			default:
				toStep := p.stepAlias(d, tr.Transition, tr.TransitionTo)
				p.writeConn(stepAlias, toStep, "-->", note)
			}
		}
	}
}

func (p *Writer) stepAlias(d *SMDecl, name string, step *MethodDecl) string {
	if step != nil {
		return fmt.Sprintf("T%02d_S%03d", d.SeqNo, step.StepNo)
	}
	p.unknownId++
	stepAlias := fmt.Sprintf("T%02d_U%03d", d.SeqNo, p.unknownId)
	p.L("state ", strconv.Quote(name), " as ", stepAlias)
	p.L(stepAlias, " : ", d.RType)
	p.L(stepAlias, " : UNKNOWN ")
	return stepAlias
}

func (p *Writer) writeConn(fromStep, toStep string, line, note string) {
	if note == "" {
		p.L(fromStep, " ", line, " ", toStep)
	} else {
		p.L(fromStep, " ", line, " ", toStep, " : ", note)
	}
}
