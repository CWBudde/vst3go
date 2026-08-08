package plugin

// #cgo CFLAGS: -I../../include
// #include "../../include/vst3/vst3_c_api.h"
// #include "../../bridge/bridge.h"
// #include "../../bridge/component.h"
// #include <stdlib.h>
// #include <string.h>
//
// // Helper functions to call IComponentHandler methods
// static inline Steinberg_tresult componentHandler_beginEdit(struct Steinberg_Vst_IComponentHandler* handler, Steinberg_Vst_ParamID id) {
//     if (handler && handler->lpVtbl && handler->lpVtbl->beginEdit) {
//         return handler->lpVtbl->beginEdit(handler, id);
//     }
//     return Steinberg_kResultFalse;
// }
//
// static inline Steinberg_tresult componentHandler_performEdit(struct Steinberg_Vst_IComponentHandler* handler, Steinberg_Vst_ParamID id, Steinberg_Vst_ParamValue value) {
//     if (handler && handler->lpVtbl && handler->lpVtbl->performEdit) {
//         return handler->lpVtbl->performEdit(handler, id, value);
//     }
//     return Steinberg_kResultFalse;
// }
//
// static inline Steinberg_tresult componentHandler_endEdit(struct Steinberg_Vst_IComponentHandler* handler, Steinberg_Vst_ParamID id) {
//     if (handler && handler->lpVtbl && handler->lpVtbl->endEdit) {
//         return handler->lpVtbl->endEdit(handler, id);
//     }
//     return Steinberg_kResultFalse;
// }
//
// // Every COM-style interface begins with the FUnknown vtable, so retain/release
// // can be reached through an opaque pointer without knowing the concrete type.
// static inline void funknown_addRef(void* obj) {
//     struct Steinberg_FUnknown* unknown = (struct Steinberg_FUnknown*)obj;
//     if (unknown && unknown->lpVtbl && unknown->lpVtbl->addRef) {
//         unknown->lpVtbl->addRef(unknown);
//     }
// }
//
// static inline void funknown_release(void* obj) {
//     struct Steinberg_FUnknown* unknown = (struct Steinberg_FUnknown*)obj;
//     if (unknown && unknown->lpVtbl && unknown->lpVtbl->release) {
//         unknown->lpVtbl->release(unknown);
//     }
// }
import "C"

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/cwbudde/vst3go/pkg/vst3"
)

// Component interface that our componentImpl satisfies
type Component interface {
	vst3.IComponent
	vst3.IAudioProcessor
	vst3.IEditController
}

// componentWrapper wraps a Go component for C callbacks
type componentWrapper struct {
	component        Component
	handle           unsafe.Pointer
	id               uintptr
	componentHandler unsafe.Pointer // IComponentHandler from host
	handlerMu        sync.RWMutex   // Protects componentHandler access
}

var (
	// componentTable maps an ID to its wrapper. IDs are dense and monotonically
	// increasing, so a slice indexed by ID replaces a hash lookup. It is
	// copy-on-write behind an atomic pointer because getComponent runs on the
	// realtime audio thread: a lock there would let a host scanner thread
	// holding the writer side stall audio (priority inversion).
	componentTable atomic.Pointer[[]*componentWrapper]

	// componentsMu serializes writers only; readers never take it.
	componentsMu sync.Mutex
	nextID       uintptr = 1
)

// Global plugin instance
var globalPlugin Plugin

// Factory info
type FactoryInfo struct {
	Vendor string
	URL    string
	Email  string
}

var globalFactoryInfo = FactoryInfo{
	Vendor: "VST3Go",
	URL:    "https://vst3go.dev",
	Email:  "info@vst3go.dev",
}

// Register sets the global plugin instance
func Register(p Plugin) {
	globalPlugin = p
}

// SetFactoryInfo sets the factory information
func SetFactoryInfo(info FactoryInfo) {
	globalFactoryInfo = info
}

// recoverPanic stops a Go panic from unwinding into the C++ host, where it
// would be undefined behavior. Use it on callbacks whose zero return value is
// a safe fallback (counts, doubles, pointers) or that return nothing.
func recoverPanic(operation string) {
	if r := recover(); r != nil {
		logPanic(operation, r)
	}
}

// recoverPanicResult is recoverPanic for callbacks returning Steinberg_tresult.
// It reports failure to the host: the zero value of tresult is kResultOk on
// non-Windows platforms, so swallowing a panic silently would claim success.
func recoverPanicResult(operation string, result *C.Steinberg_tresult) {
	if r := recover(); r != nil {
		logPanic(operation, r)
		if result != nil {
			*result = C.Steinberg_tresult(vst3.ResultFalse)
		}
	}
}

// logPanic reports a recovered panic with a stack trace. A panic crossing the
// cgo boundary is always a bug in this library; staying silent about it makes
// such bugs effectively invisible, since the host just sees wrong behavior.
func logPanic(operation string, r interface{}) {
	fmt.Fprintf(os.Stderr, "vst3go: panic in %s: %v\n%s\n", operation, r, debug.Stack())
}

// storeComponent publishes a new table with slot id set to wrapper.
// Callers must hold componentsMu.
func storeComponent(id uintptr, wrapper *componentWrapper) {
	var current []*componentWrapper
	if loaded := componentTable.Load(); loaded != nil {
		current = *loaded
	}

	size := len(current)
	if int(id)+1 > size {
		size = int(id) + 1
	}

	next := make([]*componentWrapper, size)
	copy(next, current)
	next[id] = wrapper
	componentTable.Store(&next)
}

// registerComponent registers a component wrapper and returns its ID.
// Index 0 is reserved so that a zero ID from C is always invalid.
func registerComponent(wrapper *componentWrapper) uintptr {
	componentsMu.Lock()
	defer componentsMu.Unlock()

	id := nextID
	nextID++
	wrapper.id = id
	storeComponent(id, wrapper)
	return id
}

// unregisterComponent removes a component wrapper by ID and returns it.
func unregisterComponent(id uintptr) *componentWrapper {
	componentsMu.Lock()
	defer componentsMu.Unlock()

	index := int(id)
	loaded := componentTable.Load()
	if loaded == nil || index <= 0 || index >= len(*loaded) {
		return nil
	}

	wrapper := (*loaded)[index]
	if wrapper == nil {
		return nil
	}

	storeComponent(id, nil)
	return wrapper
}

// getComponent retrieves a component wrapper by ID. This is on the audio
// callback path, so it must not lock or allocate.
func getComponent(id uintptr) *componentWrapper {
	if id == 0 {
		return nil
	}

	loaded := componentTable.Load()
	if loaded == nil {
		return nil
	}

	// int(id) can go negative for a garbage ID from C, so bound it both ways.
	index := int(id)
	table := *loaded
	if index < 0 || index >= len(table) {
		return nil
	}

	return table[index]
}

// setComponentHandler retains the host's handler and releases the one it
// replaces. VST3 requires the plugin to addRef any interface it keeps beyond
// the call; without it the host is free to destroy the handler and every later
// notifyParam* call would touch freed memory.
func (w *componentWrapper) setComponentHandler(handler unsafe.Pointer) {
	if handler != nil {
		C.funknown_addRef(handler)
	}

	w.handlerMu.Lock()
	previous := w.componentHandler
	w.componentHandler = handler
	w.handlerMu.Unlock()

	// Released unconditionally, even when it is the same object: we took a
	// second reference above, so dropping the old one leaves a net count of one.
	if previous != nil {
		C.funknown_release(previous)
	}
}

// releaseComponentHandler drops the retained handler, if any.
func (w *componentWrapper) releaseComponentHandler() {
	w.handlerMu.Lock()
	previous := w.componentHandler
	w.componentHandler = nil
	w.handlerMu.Unlock()

	if previous != nil {
		C.funknown_release(previous)
	}
}

// notifyParamBeginEdit notifies the host that parameter editing is beginning
func (w *componentWrapper) notifyParamBeginEdit(paramID uint32) {
	w.handlerMu.RLock()
	handler := w.componentHandler
	w.handlerMu.RUnlock()

	if handler == nil {
		return
	}

	// Call beginEdit through helper function
	C.componentHandler_beginEdit((*C.Steinberg_Vst_IComponentHandler)(handler), C.Steinberg_Vst_ParamID(paramID))
}

// notifyParamPerformEdit notifies the host of a parameter value change
func (w *componentWrapper) notifyParamPerformEdit(paramID uint32, valueNormalized float64) {
	w.handlerMu.RLock()
	handler := w.componentHandler
	w.handlerMu.RUnlock()

	if handler == nil {
		return
	}

	// Call performEdit through helper function
	C.componentHandler_performEdit((*C.Steinberg_Vst_IComponentHandler)(handler), C.Steinberg_Vst_ParamID(paramID), C.Steinberg_Vst_ParamValue(valueNormalized))
}

// notifyParamEndEdit notifies the host that parameter editing has ended
func (w *componentWrapper) notifyParamEndEdit(paramID uint32) {
	w.handlerMu.RLock()
	handler := w.componentHandler
	w.handlerMu.RUnlock()

	if handler == nil {
		return
	}

	// Call endEdit through helper function
	C.componentHandler_endEdit((*C.Steinberg_Vst_IComponentHandler)(handler), C.Steinberg_Vst_ParamID(paramID))
}

//export GoGetFactoryInfo
func GoGetFactoryInfo(vendor, url, email *C.char, flags *C.int32_t) {
	defer recoverPanic("GoGetFactoryInfo")

	// Buffer sizes come from Steinberg_PFactoryInfo in vst3_c_api.h.
	copyStringToChar8(vendor, 64, globalFactoryInfo.Vendor)
	copyStringToChar8(url, 256, globalFactoryInfo.URL)
	copyStringToChar8(email, 128, globalFactoryInfo.Email)
	*flags = C.Steinberg_PFactoryInfo_FactoryFlags_kUnicode
}

//export GoCountClasses
func GoCountClasses() C.int32_t {
	defer recoverPanic("GoCountClasses")

	if globalPlugin == nil {
		return 0
	}
	return 1
}

//export GoGetClassInfo
func GoGetClassInfo(index C.int32_t, cid *C.char, cardinality *C.int32_t, category, name *C.char) {
	defer recoverPanic("GoGetClassInfo")

	if globalPlugin == nil || index != 0 {
		return
	}

	info := globalPlugin.GetInfo()

	// Copy UID
	uid := info.UID()
	C.memcpy(unsafe.Pointer(cid), unsafe.Pointer(&uid[0]), 16)

	// Set cardinality
	*cardinality = C.Steinberg_PClassInfo_ClassCardinality_kManyInstances

	// Buffer sizes come from Steinberg_PClassInfo in vst3_c_api.h.
	copyStringToChar8(category, 32, "Audio Module Class")
	copyStringToChar8(name, 64, info.Name)
}

//export GoCreateInstance
func GoCreateInstance(cid *C.char, iid *C.char) unsafe.Pointer {
	defer recoverPanic("GoCreateInstance")

	if globalPlugin == nil {
		return nil
	}

	// Check if the class ID matches our plugin
	var requestedCID [16]byte
	C.memcpy(unsafe.Pointer(&requestedCID[0]), unsafe.Pointer(cid), 16)

	pluginInfo := globalPlugin.GetInfo()
	pluginUID := pluginInfo.UID()
	if requestedCID != pluginUID {
		return nil
	}

	// Create processor instance
	processor := globalPlugin.CreateProcessor()
	if processor == nil {
		return nil
	}

	// Wrap in component implementation
	component := newComponent(processor)

	// Create wrapper
	wrapper := &componentWrapper{
		component: component,
	}

	// Set wrapper reference in component for notifications
	component.wrapper = wrapper

	// Register and get ID
	id := registerComponent(wrapper)

	// Create C component with ID instead of Go pointer
	cComponent := C.createComponent(C.uintptr_t(id))
	if cComponent == nil {
		unregisterComponent(id)
		return nil
	}

	wrapper.handle = cComponent

	return cComponent
}

//export GoReleaseComponent
func GoReleaseComponent(componentID C.uintptr_t) {
	defer recoverPanic("GoReleaseComponent")

	id := uintptr(componentID)

	if id == 0 {
		return
	}

	if wrapper := unregisterComponent(id); wrapper != nil {
		wrapper.releaseComponentHandler()
	}
}

// All the IComponent callbacks
//
//export GoComponentInitialize
func GoComponentInitialize(componentID C.uintptr_t, context unsafe.Pointer) C.Steinberg_tresult {
	defer recoverPanic("GoComponentInitialize")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	err := wrapper.component.Initialize(context)
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoComponentTerminate
func GoComponentTerminate(componentID C.uintptr_t) C.Steinberg_tresult {
	defer recoverPanic("GoComponentTerminate")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	err := wrapper.component.Terminate()
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoComponentGetControllerClassId
func GoComponentGetControllerClassId(componentID C.uintptr_t, classId *C.char) {
	defer recoverPanic("GoComponentGetControllerClassId")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return
	}

	uid := wrapper.component.GetControllerClassID()
	C.memcpy(unsafe.Pointer(classId), unsafe.Pointer(&uid[0]), 16)
}

//export GoComponentSetIoMode
func GoComponentSetIoMode(componentID C.uintptr_t, mode C.int32_t) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoComponentSetIoMode", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	err := wrapper.component.SetIOMode(int32(mode))
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoComponentGetBusCount
func GoComponentGetBusCount(componentID C.uintptr_t, mediaType, dir C.int32_t) C.int32_t {
	defer recoverPanic("GoComponentGetBusCount")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return 0
	}

	return C.int32_t(wrapper.component.GetBusCount(int32(mediaType), int32(dir)))
}

//export GoComponentGetBusInfo
func GoComponentGetBusInfo(componentID C.uintptr_t, mediaType, dir, index C.int32_t, bus unsafe.Pointer) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoComponentGetBusInfo", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	info, err := wrapper.component.GetBusInfo(int32(mediaType), int32(dir), int32(index))
	if err != nil || info == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Copy bus info to C struct
	cBus := (*C.struct_Steinberg_Vst_BusInfo)(bus)
	cBus.mediaType = C.Steinberg_Vst_MediaType(info.MediaType)
	cBus.direction = C.Steinberg_Vst_BusDirection(info.Direction)
	cBus.channelCount = C.Steinberg_int32(info.ChannelCount)

	copyStringToTChar(info.Name, &cBus.name[0], len(cBus.name))

	cBus.busType = C.Steinberg_Vst_BusType(info.BusType)
	cBus.flags = C.Steinberg_uint32(info.Flags)

	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoComponentActivateBus
func GoComponentActivateBus(componentID C.uintptr_t, mediaType, dir, index, state C.int32_t) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoComponentActivateBus", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	err := wrapper.component.ActivateBus(int32(mediaType), int32(dir), int32(index), state != 0)
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoComponentSetActive
func GoComponentSetActive(componentID C.uintptr_t, state C.int32_t) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoComponentSetActive", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	err := wrapper.component.SetActive(state != 0)
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoComponentSetState
func GoComponentSetState(componentID C.uintptr_t, state unsafe.Pointer) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoComponentSetState", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Read from VST3 stream
	streamWrapper := vst3.NewStreamWrapper(state)
	if streamWrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	stateData, err := streamWrapper.ReadAll()
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Apply state to component
	if err := wrapper.component.SetState(stateData); err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoComponentGetState
func GoComponentGetState(componentID C.uintptr_t, state unsafe.Pointer) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoComponentGetState", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Get state from component
	stateData, err := wrapper.component.GetState()
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Write to VST3 stream
	streamWrapper := vst3.NewStreamWrapper(state)
	if streamWrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	_, err = streamWrapper.Write(stateData)
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	return C.Steinberg_tresult(vst3.ResultOK)
}
