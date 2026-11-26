package plugin

// #cgo CFLAGS: -I../../include
// #include "../../include/vst3/vst3_c_api.h"
import "C"

import (
	"unsafe"

	"github.com/cwbudde/vst3go/pkg/vst3"
)

// IAudioProcessor callbacks
//
//export GoAudioSetBusArrangements
func GoAudioSetBusArrangements(componentID C.uintptr_t, inputs unsafe.Pointer, numIns C.int32_t, outputs unsafe.Pointer, numOuts C.int32_t) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoAudioSetBusArrangements", &result)

	if numIns < 0 || numOuts < 0 || (numIns > 0 && inputs == nil) || (numOuts > 0 && outputs == nil) {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Convert arrangements
	var inputArrs []int64
	if numIns > 0 {
		inputArrs = unsafe.Slice((*int64)(inputs), int(numIns))
	}

	var outputArrs []int64
	if numOuts > 0 {
		outputArrs = unsafe.Slice((*int64)(outputs), int(numOuts))
	}

	err := wrapper.component.SetBusArrangements(inputArrs, outputArrs)
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoAudioGetBusArrangement
func GoAudioGetBusArrangement(componentID C.uintptr_t, dir, index C.int32_t, arr unsafe.Pointer) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoAudioGetBusArrangement", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	arrangement, err := wrapper.component.GetBusArrangement(int32(dir), int32(index))
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	*(*C.Steinberg_Vst_SpeakerArrangement)(arr) = C.Steinberg_Vst_SpeakerArrangement(arrangement)
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoAudioCanProcessSampleSize
func GoAudioCanProcessSampleSize(componentID C.uintptr_t, symbolicSampleSize C.int32_t) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoAudioCanProcessSampleSize", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	err := wrapper.component.CanProcessSampleSize(int32(symbolicSampleSize))
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoAudioGetLatencySamples
func GoAudioGetLatencySamples(componentID C.uintptr_t) C.uint32_t {
	defer recoverPanic("GoAudioGetLatencySamples")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return 0
	}

	return C.uint32_t(wrapper.component.GetLatencySamples())
}

//export GoAudioSetupProcessing
func GoAudioSetupProcessing(componentID C.uintptr_t, setup unsafe.Pointer) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoAudioSetupProcessing", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	// Convert setup
	cSetup := (*C.struct_Steinberg_Vst_ProcessSetup)(setup)
	goSetup := &vst3.ProcessSetup{
		ProcessMode:        int32(cSetup.processMode),
		SymbolicSampleSize: int32(cSetup.symbolicSampleSize),
		MaxSamplesPerBlock: int32(cSetup.maxSamplesPerBlock),
		SampleRate:         float64(cSetup.sampleRate),
	}

	err := wrapper.component.SetupProcessing(goSetup)
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoAudioSetProcessing
func GoAudioSetProcessing(componentID C.uintptr_t, state C.int32_t) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoAudioSetProcessing", &result)

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	err := wrapper.component.SetProcessing(state != 0)
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoAudioProcess
func GoAudioProcess(componentID C.uintptr_t, data unsafe.Pointer) C.Steinberg_tresult {
	defer recoverPanic("GoAudioProcess")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}

	err := wrapper.component.Process(data)
	if err != nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoAudioGetTailSamples
func GoAudioGetTailSamples(componentID C.uintptr_t) C.uint32_t {
	defer recoverPanic("GoAudioGetTailSamples")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return 0
	}

	return C.uint32_t(wrapper.component.GetTailSamples())
}
