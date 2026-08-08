package cbridge

// #cgo CFLAGS: -I../../../include
// #include "../../../bridge/bridge.h"
import "C"

import "unsafe"

// bundleEntry is the macOS module entry point. The host calls it after loading
// the bundle and before GetPluginFactory, passing the bundle's CFBundleRef.
//
// The parameter is unsafe.Pointer rather than a CoreFoundation type because
// nothing here dereferences it; taking it as an opaque pointer keeps
// CoreFoundation out of the build.
//
// The Go name is lowercase to match the symbol the host looks for. cgo exports
// the C symbol under exactly this name regardless of Go visibility.
//
//export bundleEntry
func bundleEntry(bundleRef unsafe.Pointer) bool {
	_ = bundleRef
	return C.vst3goModuleEntry() != 0
}

// bundleExit is called just before the host unloads the bundle. It must not
// free the factory: the factory is reference-counted and the host may still
// hold a reference at this point.
//
//export bundleExit
func bundleExit() bool {
	return C.vst3goModuleExit() != 0
}
