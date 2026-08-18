// Package apispec enumerates the registered connectRPC procedures by reflecting
// over the compiled protobuf descriptors. Used to seed and sync the API catalog.
package apispec

import (
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	// Blank import registers the generated descriptors into protoregistry. It
	// must stay here unconditionally so the registry is populated even when this
	// package is exercised in isolation (e.g. its own unit test), which does not
	// otherwise compile the service package.
	_ "github.com/zerx-lab/zkit/gen/go/zerx/v1"
)

// Proc identifies one RPC procedure.
type Proc struct {
	Procedure string
	Service   string
	Method    string
}

// Procedures returns every zerx.v1 RPC procedure in connectRPC path form
// (/zerx.v1.<Service>/<Method>).
func Procedures() []Proc {
	var procs []Proc
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		services := fd.Services()
		for i := range services.Len() {
			svc := services.Get(i)
			if !strings.HasPrefix(string(svc.FullName()), "zerx.v1.") {
				continue
			}
			methods := svc.Methods()
			for j := range methods.Len() {
				m := methods.Get(j)
				procs = append(procs, Proc{
					Procedure: "/" + string(svc.FullName()) + "/" + string(m.Name()),
					Service:   string(svc.FullName()),
					Method:    string(m.Name()),
				})
			}
		}

		return true
	})

	return procs
}
