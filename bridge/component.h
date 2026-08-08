#ifndef VST3GO_COMPONENT_H
#define VST3GO_COMPONENT_H

#include "../include/vst3/vst3_c_api.h"
#include <stdint.h>

// C function to create a component wrapper
void* createComponent(uintptr_t goComponent);

// Go callback declarations for IComponent
extern Steinberg_tresult GoComponentInitialize(uintptr_t component, void* context);
extern Steinberg_tresult GoComponentTerminate(uintptr_t component);
extern void GoComponentGetControllerClassId(uintptr_t component, char* classId);
extern Steinberg_tresult GoComponentSetIoMode(uintptr_t component, int32_t mode);
extern int32_t GoComponentGetBusCount(uintptr_t component, int32_t type, int32_t dir);
extern Steinberg_tresult GoComponentGetBusInfo(uintptr_t component, int32_t type, int32_t dir, int32_t index, void* bus);
extern Steinberg_tresult GoComponentActivateBus(uintptr_t component, int32_t type, int32_t dir, int32_t index, int32_t state);
extern Steinberg_tresult GoComponentSetActive(uintptr_t component, int32_t state);
extern Steinberg_tresult GoComponentSetState(uintptr_t component, void* state);
extern Steinberg_tresult GoComponentGetState(uintptr_t component, void* state);

// Go callback declarations for IAudioProcessor
extern Steinberg_tresult GoAudioSetBusArrangements(uintptr_t component, void* inputs, int32_t numIns, void* outputs, int32_t numOuts);
extern Steinberg_tresult GoAudioGetBusArrangement(uintptr_t component, int32_t dir, int32_t index, void* arr);
extern Steinberg_tresult GoAudioCanProcessSampleSize(uintptr_t component, int32_t symbolicSampleSize);
extern uint32_t GoAudioGetLatencySamples(uintptr_t component);
extern Steinberg_tresult GoAudioSetupProcessing(uintptr_t component, void* setup);
extern Steinberg_tresult GoAudioSetProcessing(uintptr_t component, int32_t state);
extern Steinberg_tresult GoAudioProcess(uintptr_t component, void* data);
extern uint32_t GoAudioGetTailSamples(uintptr_t component);

// Go callback declarations for IEditController
extern Steinberg_tresult GoEditControllerSetComponentState(uintptr_t component, void* state);
extern Steinberg_tresult GoEditControllerSetState(uintptr_t component, void* state);
extern Steinberg_tresult GoEditControllerGetState(uintptr_t component, void* state);
extern int32_t GoEditControllerGetParameterCount(uintptr_t component);
extern Steinberg_tresult GoEditControllerGetParameterInfo(uintptr_t component, int32_t paramIndex, struct Steinberg_Vst_ParameterInfo* info);
extern Steinberg_tresult GoEditControllerGetParamStringByValue(uintptr_t component, Steinberg_Vst_ParamID id, Steinberg_Vst_ParamValue valueNormalized, Steinberg_Vst_TChar* string);
extern Steinberg_tresult GoEditControllerGetParamValueByString(uintptr_t component, Steinberg_Vst_ParamID id, Steinberg_Vst_TChar* string, Steinberg_Vst_ParamValue* valueNormalized);
extern double GoEditControllerNormalizedParamToPlain(uintptr_t component, uint32_t id, double valueNormalized);
extern double GoEditControllerPlainParamToNormalized(uintptr_t component, uint32_t id, double plainValue);
extern double GoEditControllerGetParamNormalized(uintptr_t component, uint32_t id);
extern Steinberg_tresult GoEditControllerSetParamNormalized(uintptr_t component, uint32_t id, double value);
extern Steinberg_tresult GoEditControllerSetComponentHandler(uintptr_t component, void* handler);
extern void* GoEditControllerCreateView(uintptr_t component, char* name);

// Go component lifecycle
extern void GoReleaseComponent(uintptr_t component);

#endif // VST3GO_COMPONENT_H