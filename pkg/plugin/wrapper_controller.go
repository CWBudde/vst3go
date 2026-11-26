package plugin

// #cgo CFLAGS: -I../../include
// #include "../../include/vst3/vst3_c_api.h"
import "C"

import (
	"bytes"
	"unsafe"

	"github.com/cwbudde/vst3go/pkg/vst3"
)

// IEditController callbacks
//
//export GoEditControllerSetComponentState
func GoEditControllerSetComponentState(componentID C.uintptr_t, state unsafe.Pointer) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoEditControllerSetComponentState", &result)
	if wrapper := getComponent(uintptr(componentID)); wrapper != nil {
		wrapper.traceHostCall("IEditController.setComponentState", 0, 0, 0, 0)
	}

	// Component state received from processor - apply to edit controller
	return GoComponentSetState(componentID, state)
}

//export GoEditControllerSetState
func GoEditControllerSetState(componentID C.uintptr_t, state unsafe.Pointer) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoEditControllerSetState", &result)
	if wrapper := getComponent(uintptr(componentID)); wrapper != nil {
		wrapper.traceHostCall("IEditController.setState", 0, 0, 0, 0)
	}

	if provider, ok := getEditorStateProvider(uintptr(componentID)); ok {
		data, err := vst3.NewStreamWrapper(state).ReadAll()
		if err != nil || provider.LoadEditorState(bytes.NewReader(data)) != nil {
			return C.Steinberg_tresult(vst3.ResultFalse)
		}
		return C.Steinberg_tresult(vst3.ResultOK)
	}
	return GoComponentSetState(componentID, state)
}

//export GoEditControllerGetState
func GoEditControllerGetState(componentID C.uintptr_t, state unsafe.Pointer) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoEditControllerGetState", &result)
	if wrapper := getComponent(uintptr(componentID)); wrapper != nil {
		wrapper.traceHostCall("IEditController.getState", 0, 0, 0, 0)
	}

	if provider, ok := getEditorStateProvider(uintptr(componentID)); ok {
		var data bytes.Buffer
		if err := provider.SaveEditorState(&data); err != nil {
			return C.Steinberg_tresult(vst3.ResultFalse)
		}
		if _, err := vst3.NewStreamWrapper(state).Write(data.Bytes()); err != nil {
			return C.Steinberg_tresult(vst3.ResultFalse)
		}
		return C.Steinberg_tresult(vst3.ResultOK)
	}
	return GoComponentGetState(componentID, state)
}

func getEditorStateProvider(componentID uintptr) (EditorStateProvider, bool) {
	wrapper := getComponent(componentID)
	if wrapper != nil {
		if component, ok := wrapper.component.(*componentImpl); ok {
			state, supported := component.processor.(EditorStateProvider)
			return state, supported
		}
	}
	return nil, false
}

//export GoEditControllerGetParameterCount
func GoEditControllerGetParameterCount(componentID C.uintptr_t) C.int32_t {
	defer recoverPanic("GoEditControllerGetParameterCount")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return 0
	}

	return C.int32_t(wrapper.component.GetParameterCount())
}

//export GoEditControllerGetParameterInfo
func GoEditControllerGetParameterInfo(componentID C.uintptr_t, paramIndex C.int32_t, info *C.struct_Steinberg_Vst_ParameterInfo) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoEditControllerGetParameterInfo", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	paramInfo, err := wrapper.component.GetParameterInfo(int32(paramIndex))
	if err != nil || paramInfo == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Copy to C struct
	cInfo := info
	cInfo.id = C.Steinberg_Vst_ParamID(paramInfo.ID)

	copyStringToTChar(paramInfo.Title, &cInfo.title[0], len(cInfo.title))
	copyStringToTChar(paramInfo.ShortTitle, &cInfo.shortTitle[0], len(cInfo.shortTitle))
	copyStringToTChar(paramInfo.Units, &cInfo.units[0], len(cInfo.units))

	cInfo.stepCount = C.Steinberg_int32(paramInfo.StepCount)
	cInfo.defaultNormalizedValue = C.Steinberg_Vst_ParamValue(paramInfo.DefaultValue)
	cInfo.unitId = C.Steinberg_Vst_UnitID(paramInfo.UnitID)
	cInfo.flags = C.Steinberg_int32(paramInfo.Flags)

	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoEditControllerGetParamStringByValue
func GoEditControllerGetParamStringByValue(componentID C.uintptr_t, id C.Steinberg_Vst_ParamID, valueNormalized C.Steinberg_Vst_ParamValue, string *C.Steinberg_Vst_TChar) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoEditControllerGetParamStringByValue", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Get the formatted string
	str, err := wrapper.component.GetParamStringByValue(uint32(id), float64(valueNormalized))
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Convert to UTF16 for VST3
	copyStringToTChar(str, string, 128)

	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoEditControllerGetParamValueByString
func GoEditControllerGetParamValueByString(componentID C.uintptr_t, id C.Steinberg_Vst_ParamID, string *C.Steinberg_Vst_TChar, valueNormalized *C.Steinberg_Vst_ParamValue) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoEditControllerGetParamValueByString", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Convert from UTF16
	str := stringFromTChar(string)

	// Parse the value
	value, err := wrapper.component.GetParamValueByString(uint32(id), str)
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	*valueNormalized = C.Steinberg_Vst_ParamValue(value)
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoEditControllerNormalizedParamToPlain
func GoEditControllerNormalizedParamToPlain(componentID C.uintptr_t, id C.uint32_t, valueNormalized C.double) C.double {
	defer recoverPanic("GoEditControllerNormalizedParamToPlain")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return valueNormalized
	}

	return C.double(wrapper.component.NormalizedParamToPlain(uint32(id), float64(valueNormalized)))
}

//export GoEditControllerPlainParamToNormalized
func GoEditControllerPlainParamToNormalized(componentID C.uintptr_t, id C.uint32_t, plainValue C.double) C.double {
	defer recoverPanic("GoEditControllerPlainParamToNormalized")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return plainValue
	}

	return C.double(wrapper.component.PlainParamToNormalized(uint32(id), float64(plainValue)))
}

//export GoEditControllerGetParamNormalized
func GoEditControllerGetParamNormalized(componentID C.uintptr_t, id C.uint32_t) C.double {
	defer recoverPanic("GoEditControllerGetParamNormalized")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return 0
	}

	return C.double(wrapper.component.GetParamNormalized(uint32(id)))
}

//export GoEditControllerSetParamNormalized
func GoEditControllerSetParamNormalized(componentID C.uintptr_t, id C.uint32_t, value C.double) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoEditControllerSetParamNormalized", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	err := wrapper.component.SetParamNormalized(uint32(id), float64(value))
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoEditControllerSetComponentHandler
func GoEditControllerSetComponentHandler(componentID C.uintptr_t, handler unsafe.Pointer) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoEditControllerSetComponentHandler", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Store the component handler for parameter change notifications
	wrapper.traceHostCall("IEditController.setComponentHandler", hostFlag(handler != nil), 0, 0, 0)
	wrapper.setComponentHandler(handler)

	return C.Steinberg_tresult(vst3.ResultOK)
}
