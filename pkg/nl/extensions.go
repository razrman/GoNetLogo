package nl

// Extensions are loaded from the extensions declaration. A primitive is
// visible only after its extension is declared, the same rule NetLogo uses.

func (p *Program) hasExtension(name string) bool {
	if p == nil {
		return false
	}
	for _, e := range p.Extensions {
		if e == name {
			return true
		}
	}
	return false
}

func (p *parser) installExtensions() {
	if p.prog == nil {
		return
	}
	seen := map[string]bool{}
	for _, name := range p.prog.Extensions {
		if seen[name] {
			continue
		}
		seen[name] = true
		switch name {
		case "palette":
			p.installPalette()
		case "sr":
			p.installSR()
		}
	}
}

func (p *parser) addReporters(m map[string]primSpec) {
	if p.extraRep == nil {
		p.extraRep = map[string]primSpec{}
	}
	for name, spec := range m {
		p.extraRep[name] = spec
	}
}

func (p *parser) addCommands(m map[string]primSpec) {
	if p.extraCmd == nil {
		p.extraCmd = map[string]primSpec{}
	}
	for name, spec := range m {
		p.extraCmd[name] = spec
	}
}

func (p *parser) addInfix(m map[string]infixSpec) {
	if p.extraInfix == nil {
		p.extraInfix = map[string]infixSpec{}
	}
	for name, spec := range m {
		p.extraInfix[name] = spec
	}
}

func isExtZeroArg(prog *Program, name string) bool {
	if prog == nil {
		return false
	}
	if prog.hasExtension("palette") && paletteZero[name] {
		return true
	}
	if prog.hasExtension("sr") && srZero[name] {
		return true
	}
	return false
}

func isExtCommand(prog *Program, name string) bool {
	if prog == nil {
		return false
	}
	if prog.hasExtension("palette") {
		if _, ok := paletteCommands[name]; ok {
			return true
		}
	}
	if prog.hasExtension("sr") {
		if _, ok := srCommands[name]; ok {
			return true
		}
	}
	return false
}

func (vm *VM) extCommand(name string, args []Value, pos Pos) (error, bool) {
	switch {
	case len(name) >= 8 && name[:8] == "palette:":
		return vm.paletteCommand(name, args, pos)
	case len(name) >= 3 && name[:3] == "sr:":
		return vm.srCommand(name, args, pos)
	default:
		return nil, false
	}
}

func (vm *VM) extReporter(name string, args []Value, pos Pos) (Value, bool, error) {
	switch {
	case len(name) >= 8 && name[:8] == "palette:":
		v, err := vm.paletteReporter(name, args, pos)
		return v, true, err
	case len(name) >= 3 && name[:3] == "sr:":
		v, err := vm.srReporter(name, args, pos)
		return v, true, err
	default:
		return Value{}, false, nil
	}
}

func (vm *VM) extZero(name string, pos Pos) (Value, bool, error) {
	if paletteZero[name] && vm.Prog != nil && vm.Prog.hasExtension("palette") {
		v, err := vm.paletteZero(name, pos)
		return v, true, err
	}
	if srZero[name] && vm.Prog != nil && vm.Prog.hasExtension("sr") {
		v, err := vm.srZero(name, pos)
		return v, true, err
	}
	return Value{}, false, nil
}
