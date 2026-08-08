package cbridge

// This file defines the one symbol every VST3 host requires a module to export.
//
// It is a Go //export rather than a plain C function on purpose. A shared
// library's export table is not populated the same way on the three target
// platforms, and only //export covers all of them:
//
//   - ELF (Linux): a C function with default visibility is exported, and so is
//     a //export function — either works.
//   - Mach-O (macOS): likewise.
//   - PE (Windows): it does NOT work. For a c-shared build the Go linker
//     generates an explicit .def file whose EXPORTS section is built solely
//     from //export symbols and passes it to ld, which then exports exactly
//     that set. A C-side __declspec(dllexport) is dropped without a diagnostic,
//     so the DLL links cleanly and every host reports it as an invalid module.
//
// Defining the entry points here therefore removes a whole class of
// platform-specific build machinery instead of adding some, which is why the C
// bridge no longer carries any #ifdef for this.

// #cgo CFLAGS: -I../../../include
// #include "../../../bridge/bridge.h"
import "C"

import "unsafe"

// GetPluginFactory is the VST3 module entry point. Hosts resolve it by name
// (dlsym/GetProcAddress) immediately after loading the module and call it to
// obtain the IPluginFactory that enumerates and instantiates the plugin's
// classes.
//
// The result is an IPluginFactory*; it is typed as unsafe.Pointer because the
// only thing this side does with it is hand it straight back to the host.
//
// Threading: called on the host's module-loading thread, which is not the audio
// thread. It may allocate.
//
//export GetPluginFactory
func GetPluginFactory() unsafe.Pointer {
	return unsafe.Pointer(C.vst3goGetPluginFactory())
}
