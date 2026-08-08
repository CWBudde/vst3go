package cbridge

// #cgo CFLAGS: -I../../../include
// #include "../../../bridge/bridge.h"
import "C"

// InitDll is the Windows module entry point. The SDK's module loader resolves
// it by name after LoadLibrary and, if present, calls it before
// GetPluginFactory. It is optional — GetPluginFactory is the only mandatory
// export — but hosts that call one call the other, so the pair is defined
// together.
//
// Returning false tells the host the module failed to initialize and it will
// unload it.
//
//export InitDll
func InitDll() bool {
	return C.vst3goModuleEntry() != 0
}

// ExitDll is called just before the host unloads the module. It must not free
// the factory: the factory is reference-counted and the host may still hold a
// reference at this point.
//
//export ExitDll
func ExitDll() bool {
	return C.vst3goModuleExit() != 0
}
