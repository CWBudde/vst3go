package plugin

// #cgo CFLAGS: -I../../include
// #include "../../bridge/program.h"
import "C"

import "github.com/cwbudde/vst3go/pkg/vst3"

func getFactoryPrograms(componentID C.uintptr_t) *factoryPrograms {
	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return nil
	}
	component, ok := wrapper.component.(*componentImpl)
	if !ok {
		return nil
	}
	return component.programs
}

//export GoFactoryProgramListCount
func GoFactoryProgramListCount(componentID C.uintptr_t) C.Steinberg_int32 {
	defer recoverPanic("GoFactoryProgramListCount")
	if getFactoryPrograms(componentID) != nil {
		return 1
	}
	return 0
}

//export GoFactoryProgramListInfo
func GoFactoryProgramListInfo(componentID C.uintptr_t, info *C.struct_Steinberg_Vst_ProgramListInfo) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoFactoryProgramListInfo", &result)
	programs := getFactoryPrograms(componentID)
	if programs == nil || info == nil {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	*info = C.struct_Steinberg_Vst_ProgramListInfo{
		id:           C.Steinberg_Vst_ProgramListID(programs.list.ID),
		programCount: C.Steinberg_int32(len(programs.list.Names)),
	}
	copyStringToTChar(programs.list.Name, &info.name[0], len(info.name))
	return C.Steinberg_tresult(vst3.ResultOK)
}

//export GoFactoryProgramName
func GoFactoryProgramName(componentID C.uintptr_t, listID C.Steinberg_Vst_ProgramListID, program C.Steinberg_int32, name *C.Steinberg_Vst_TChar) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoFactoryProgramName", &result)
	programs := getFactoryPrograms(componentID)
	if programs == nil || name == nil || int32(listID) != programs.list.ID || program < 0 || int(program) >= len(programs.list.Names) {
		return C.Steinberg_tresult(vst3.ResultFalse)
	}
	copyStringToTChar(programs.list.Names[program], name, 128)
	return C.Steinberg_tresult(vst3.ResultOK)
}
