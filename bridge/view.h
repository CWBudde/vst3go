#ifndef VST3GO_VIEW_H
#define VST3GO_VIEW_H

#include "../include/vst3/vst3_c_api.h"
#include <stdint.h>

Steinberg_IPlugView* createPlugView(Steinberg_Vst_IEditController* owner, uintptr_t component);
Steinberg_tresult requestPlugViewResize(void* view, struct Steinberg_ViewRect* size);
void* startPlugViewTimer(void* view, uintptr_t id, Steinberg_Linux_TimerInterval interval);
void stopPlugViewTimer(void* timer);

extern uintptr_t GoEditControllerCreateView(uintptr_t component, void* view, Steinberg_TBool* scalable);
extern void GoPlugViewDestroy(uintptr_t view);
extern Steinberg_tresult GoPlugViewSupportsPlatform(uintptr_t view, char* type);
extern Steinberg_tresult GoPlugViewAttach(uintptr_t view, uintptr_t parent, char* type);
extern Steinberg_tresult GoPlugViewRemove(uintptr_t view);
extern Steinberg_tresult GoPlugViewWheel(uintptr_t view, float distance);
extern Steinberg_tresult GoPlugViewKey(uintptr_t view, Steinberg_char16 key, Steinberg_int16 code,
                                       Steinberg_int16 modifiers, Steinberg_TBool down);
extern Steinberg_tresult GoPlugViewGetSize(uintptr_t view, struct Steinberg_ViewRect* size);
extern Steinberg_tresult GoPlugViewResize(uintptr_t view, struct Steinberg_ViewRect* size);
extern Steinberg_tresult GoPlugViewFocus(uintptr_t view, Steinberg_TBool focused);
extern Steinberg_tresult GoPlugViewCanResize(uintptr_t view);
extern Steinberg_tresult GoPlugViewConstrainSize(uintptr_t view, struct Steinberg_ViewRect* size);
extern Steinberg_tresult GoPlugViewScale(uintptr_t view, float factor);
extern void GoPlugViewTimerTick(uintptr_t id);

#endif
