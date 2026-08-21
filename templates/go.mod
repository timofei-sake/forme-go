module github.com/formepdf/forme-go/templates

// Inherited from wazero's own floor. It also keeps this module away from the
// toolchains that could not run the engine: Go 1.24 and older now refuse to
// build it rather than trapping at render time (see the requirement below).
go 1.25.0

// Never go below v1.9.0. Go 1.24 rewrote memmove on amd64 (golang/go
// 601ea46a) so that its non-AVX path clobbers DX, a register wazero's amd64
// compiler did not reserve across the call. Renders then trap with "out of
// bounds memory access" whenever memmove takes that path - which depends on
// the CPU's reported features, so it hits some machines and not others on
// every Go release from 1.24 onward. tetratelabs/wazero#2378 reserves DX and
// shipped in v1.9.0. Reproduce the old failure with GODEBUG=cpu.all=off,
// which forces the offending memmove path regardless of hardware.
require github.com/tetratelabs/wazero v1.12.0

require golang.org/x/sys v0.44.0 // indirect
