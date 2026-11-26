#ifndef VST3GO_PROGRAM_H
#define VST3GO_PROGRAM_H

#include <stdint.h>
#include <vst3/vst3_c_api.h>

typedef struct {
    Steinberg_Vst_IUnitInfo iface;
    Steinberg_FUnknown* owner;
    uintptr_t componentID;
} ProgramInfoInterface;

void initProgramInfo(ProgramInfoInterface* unit, Steinberg_FUnknown* owner, uintptr_t componentID);

extern Steinberg_int32 GoFactoryProgramListCount(uintptr_t componentID);
extern Steinberg_tresult GoFactoryProgramListInfo(uintptr_t componentID, struct Steinberg_Vst_ProgramListInfo* info);
extern Steinberg_tresult GoFactoryProgramName(uintptr_t componentID, Steinberg_Vst_ProgramListID listID,
                                             Steinberg_int32 program, Steinberg_Vst_TChar* name);

#endif
