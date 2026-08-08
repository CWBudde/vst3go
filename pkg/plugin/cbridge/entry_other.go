//go:build !windows && !darwin

package cbridge

// #cgo CFLAGS: -I../../../include
// #include "../../../bridge/bridge.h"
import "C"

import "unsafe"

// ModuleEntry is the Linux module entry point (and the one the VST3 spec
// defines for any other ELF platform). The host calls it after dlopen and
// before GetPluginFactory, passing the dlopen handle.
//
// The handle is taken as an opaque pointer and ignored: the Go runtime is
// already initialized by the shared library's own init, so there is nothing to
// bootstrap here.
//
//export ModuleEntry
func ModuleEntry(sharedLibraryHandle unsafe.Pointer) bool {
	_ = sharedLibraryHandle
	return C.vst3goModuleEntry() != 0
}

// ModuleExit is called just before the host unloads the module. It must not
// free the factory: the factory is reference-counted and the host may still
// hold a reference at this point.
//
//export ModuleExit
func ModuleExit() bool {
	return C.vst3goModuleExit() != 0
}
