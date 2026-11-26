package plugin

// #cgo CFLAGS: -I../../include
// #include <vst3/vst3_c_api.h>
import "C"

import (
	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/vst3"
)

func getMIDIControllerMapper(componentID C.uintptr_t) MIDIControllerMapper {
	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return nil
	}
	component, ok := wrapper.component.(*componentImpl)
	if !ok {
		return nil
	}
	mapper, _ := component.processor.(MIDIControllerMapper)
	return mapper
}

//export GoHasMIDIControllerMapping
func GoHasMIDIControllerMapping(componentID C.uintptr_t) C.int32_t {
	defer recoverPanic("GoHasMIDIControllerMapping")
	if getMIDIControllerMapper(componentID) != nil {
		return 1
	}
	return 0
}

//export GoGetMIDIControllerAssignment
func GoGetMIDIControllerAssignment(componentID C.uintptr_t, busIndex C.int32_t, channel, controller C.int16_t, id *C.Steinberg_Vst_ParamID) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoGetMIDIControllerAssignment", &result)
	if id == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	mapper := getMIDIControllerMapper(componentID)
	if mapper == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	buses := mapper.GetBuses()
	index := int32(busIndex)
	if buses == nil || index < 0 || index >= buses.GetBusCount(bus.MediaTypeEvent, bus.DirectionInput) ||
		channel < 0 || channel >= 16 || controller < 0 || controller >= C.Steinberg_Vst_ControllerNumbers_kCountCtrlNumber {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	parameter, ok := mapper.MIDIControllerAssignment(index, int16(channel), int16(controller))
	params := mapper.GetParameters()
	if !ok || params == nil || parameter == ^uint32(0) || params.Get(parameter) == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	*id = C.Steinberg_Vst_ParamID(parameter)
	return C.Steinberg_tresult(vst3.ResultOK)
}
