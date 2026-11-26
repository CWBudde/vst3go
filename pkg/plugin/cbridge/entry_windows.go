package cbridge

// #cgo CFLAGS: -I../../../include
// #include "../../../bridge/bridge.h"
// #include <windows.h>
//
// static inline BOOL pinModule(void) {
//     static const char anchor = 0;
//     HMODULE module;
//     return GetModuleHandleExW(GET_MODULE_HANDLE_EX_FLAG_PIN |
//                              GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS,
//                              (LPCWSTR)&anchor, &module);
// }
import "C"

// prepareModule keeps the Go runtime's code mapped after the host calls
// FreeLibrary. Runtime threads and vectored exception handlers can outlive
// ExitDll.
func prepareModule() bool {
	return C.pinModule() != 0
}

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
	return prepareModule() && C.vst3goModuleEntry() != 0
}

// ExitDll is called just before the host unloads the module. It must not free
// the factory: the factory is reference-counted and the host may still hold a
// reference at this point.
//
//export ExitDll
func ExitDll() bool {
	return C.vst3goModuleExit() != 0
}
