#include <vst3/vst3_c_api.h>
#include <assert.h>
#include <stddef.h>
#include "state_stream.h"
#include <stdint.h>

extern struct Steinberg_IPluginFactory* GetPluginFactory(void);

typedef struct {
    Steinberg_IPlugFrame iface;
    unsigned references;
    unsigned resizes;
    struct Steinberg_ViewRect beforeResize;
} HostFrame;

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE frame_add_ref(void* self) {
    return ++((HostFrame*)self)->references;
}

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE frame_release(void* self) {
    return --((HostFrame*)self)->references;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE frame_resize(
    void* self, Steinberg_IPlugView* view, struct Steinberg_ViewRect* size) {
    HostFrame* frame = self;
    frame->resizes++;
    assert(view->lpVtbl->getSize(view, &frame->beforeResize) == Steinberg_kResultOk);
    return view->lpVtbl->onSize(view, size);
}

static Steinberg_IPlugFrameVtbl frame_vtable = {
    .addRef = frame_add_ref,
    .release = frame_release,
    .resizeView = frame_resize,
};

typedef struct {
    Steinberg_Vst_IComponentHandler iface;
    unsigned references;
    unsigned steps;
    unsigned restarts;
    Steinberg_Vst_IAudioProcessor* audio;
    Steinberg_uint32 latency;
} HostHandler;

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE handler_add_ref(void* self) {
    return ++((HostHandler*)self)->references;
}

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE handler_release(void* self) {
    return --((HostHandler*)self)->references;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE handler_begin(void* self, Steinberg_Vst_ParamID id) {
    HostHandler* handler = self;
    assert(id == 0 && handler->steps == 0 && handler->references == 3);
    handler->steps++;
    return Steinberg_kResultOk;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE handler_perform(void* self, Steinberg_Vst_ParamID id,
                                                               Steinberg_Vst_ParamValue normalized) {
    HostHandler* handler = self;
    assert(id == 0 && handler->references == 3);
    assert((handler->steps == 1 && normalized == 0.5) || (handler->steps == 2 && normalized == 0.75));
    handler->steps++;
    return Steinberg_kResultOk;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE handler_end(void* self, Steinberg_Vst_ParamID id) {
    HostHandler* handler = self;
    assert(id == 0 && handler->steps == 3 && handler->references == 3);
    handler->steps++;
    return Steinberg_kResultOk;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE handler_restart(void* self, Steinberg_int32 flags) {
    HostHandler* handler = self;
    assert(flags == Steinberg_Vst_RestartFlags_kLatencyChanged && handler->references == 3);
    handler->latency = handler->audio->lpVtbl->getLatencySamples(handler->audio);
    handler->restarts++;
    return Steinberg_kResultOk;
}

static Steinberg_Vst_IComponentHandlerVtbl handler_vtable = {
    .addRef = handler_add_ref,
    .release = handler_release,
    .beginEdit = handler_begin,
    .performEdit = handler_perform,
    .endEdit = handler_end,
    .restartComponent = handler_restart,
};

static Steinberg_Vst_IEditController* create_controller(Steinberg_IPluginFactory* factory, int index) {
    struct Steinberg_PClassInfo info;
    Steinberg_Vst_IEditController* controller = NULL;
    assert(factory->lpVtbl->getClassInfo(factory, index, &info) == Steinberg_kResultOk);
    assert(factory->lpVtbl->createInstance(factory, info.cid, Steinberg_Vst_IEditController_iid,
        (void**)&controller) == Steinberg_kResultOk);
    return controller;
}

static void check_size(Steinberg_IPlugView* view, int width, int height) {
    struct Steinberg_ViewRect size = {0};
    assert(view->lpVtbl->getSize(view, &size) == Steinberg_kResultOk);
    assert(size.right - size.left == width && size.bottom - size.top == height);
}

static void check_programs(Steinberg_IPluginFactory* factory, Steinberg_Vst_IEditController* generic) {
    Steinberg_Vst_IUnitInfo* units = NULL;
    assert(generic->lpVtbl->queryInterface(generic, Steinberg_Vst_IUnitInfo_iid, (void**)&units) != Steinberg_kResultOk);
    assert(units == NULL);

    Steinberg_Vst_IEditController* controller = create_controller(factory, 2);
    assert(controller->lpVtbl->queryInterface(controller, Steinberg_Vst_IUnitInfo_iid, (void**)&units) == Steinberg_kResultOk);
    assert(units->lpVtbl->getUnitCount(units) == 1 && units->lpVtbl->getProgramListCount(units) == 1);
    struct Steinberg_Vst_UnitInfo unit;
    assert(units->lpVtbl->getUnitInfo(units, 0, &unit) == Steinberg_kResultOk);
    assert(unit.id == Steinberg_Vst_kRootUnitId && unit.parentUnitId == Steinberg_Vst_kNoParentUnitId);
    struct Steinberg_Vst_ProgramListInfo list;
    assert(units->lpVtbl->getProgramListInfo(units, 0, &list) == Steinberg_kResultOk);
    assert(list.id == 11 && list.programCount == 2 && list.id == unit.programListId);
    Steinberg_Vst_String128 name;
    assert(units->lpVtbl->getProgramName(units, list.id, 0, name) == Steinberg_kResultOk);
    assert(name[0] == 'M' && name[1] == 0x00FC && name[5] == 'r' && name[6] == 0);
    assert(units->lpVtbl->getProgramName(units, list.id, 2, name) != Steinberg_kResultOk);
    assert(units->lpVtbl->getProgramName(units, list.id + 1, 0, name) != Steinberg_kResultOk);

    struct Steinberg_Vst_ParameterInfo parameter;
    assert(controller->lpVtbl->getParameterCount(controller) == 2);
    assert(controller->lpVtbl->getParameterInfo(controller, 1, &parameter) == Steinberg_kResultOk);
    assert(parameter.id == 9 && parameter.unitId == unit.id && parameter.stepCount == 1);
    assert((parameter.flags & Steinberg_Vst_ParameterInfo_ParameterFlags_kIsProgramChange) != 0);
    assert((parameter.flags & Steinberg_Vst_ParameterInfo_ParameterFlags_kCanAutomate) != 0);
    assert(parameter.defaultNormalizedValue == 1.0);
    assert(controller->lpVtbl->getParamNormalized(controller, 9) == 1.0);
    assert(controller->lpVtbl->setParamNormalized(controller, 9, 0.0) == Steinberg_kResultOk);
    assert(controller->lpVtbl->getParamNormalized(controller, 0) == 0.25);

    StateStream saved = {.stream = {&state_vtbl}};
    assert(controller->lpVtbl->setParamNormalized(controller, 0, 0.375) == Steinberg_kResultOk);
    assert(controller->lpVtbl->getState(controller, &saved.stream) == Steinberg_kResultOk);
    assert(controller->lpVtbl->setParamNormalized(controller, 9, 1.0) == Steinberg_kResultOk);
    assert(controller->lpVtbl->getParamNormalized(controller, 0) == 1.0);
    saved.cursor = 0;
    assert(controller->lpVtbl->setState(controller, &saved.stream) == Steinberg_kResultOk);
    assert(controller->lpVtbl->getParamNormalized(controller, 9) == 0.0);
    assert(controller->lpVtbl->getParamNormalized(controller, 0) == 0.375);

    void* component = NULL;
    assert(units->lpVtbl->queryInterface(units, Steinberg_FUnknown_iid, &component) == Steinberg_kResultOk);
    Steinberg_FUnknown* identity = component;
    identity->lpVtbl->release(identity);
    units->lpVtbl->release(units);
    assert(controller->lpVtbl->release(controller) == 0);
}

int main(void) {
    Steinberg_IPluginFactory* factory = GetPluginFactory();
    Steinberg_Vst_IEditController* controller = create_controller(factory, 0);
    assert(controller->lpVtbl->createView(controller, "unknown") == NULL);
    assert(controller->lpVtbl->createView(controller, NULL) == NULL);

    Steinberg_IPlugView* view = controller->lpVtbl->createView(controller, Steinberg_Vst_ViewType_kEditor);
    assert(view && "processor that provides an editor must publish IPlugView");
    assert(view->lpVtbl->isPlatformTypeSupported(view, Steinberg_kPlatformTypeX11EmbedWindowID) == Steinberg_kResultOk);
    assert(view->lpVtbl->isPlatformTypeSupported(view, Steinberg_kPlatformTypeHWND) == Steinberg_kResultFalse);
    void* identity = NULL;
    assert(view->lpVtbl->queryInterface(view, Steinberg_FUnknown_iid, &identity) == Steinberg_kResultOk);
    assert(identity == view);
    view->lpVtbl->release(identity);
    assert(view->lpVtbl->queryInterface(view, Steinberg_Vst_IComponent_iid, &identity) == Steinberg_kNoInterface);
    assert(identity == NULL);

    Steinberg_IPlugViewContentScaleSupport* scale = NULL;
    assert(view->lpVtbl->queryInterface(view, Steinberg_IPlugViewContentScaleSupport_iid,
        (void**)&scale) == Steinberg_kResultOk);
    assert(scale->lpVtbl->queryInterface(scale, Steinberg_FUnknown_iid, &identity) == Steinberg_kResultOk);
    assert(identity == view);
    view->lpVtbl->release(identity);

    // A small X11 Window ID is a numeric handle, never a Go pointer.
    void* parent = (void*)(uintptr_t)123;
    assert(view->lpVtbl->attached(view, parent, Steinberg_kPlatformTypeX11EmbedWindowID) == Steinberg_kResultFalse);
    HostFrame frame = {{&frame_vtable}, 1, 0, {0}};
    assert(view->lpVtbl->setFrame(view, &frame.iface) == Steinberg_kResultOk);
    assert(view->lpVtbl->setFrame(view, &frame.iface) == Steinberg_kResultOk);
    assert(frame.references == 2);
    assert(view->lpVtbl->canResize(view) == Steinberg_kResultOk);
    check_size(view, 300, 200);

    assert(view->lpVtbl->attached(view, parent, Steinberg_kPlatformTypeHWND) == Steinberg_kResultFalse);
    assert(view->lpVtbl->attached(view, NULL, Steinberg_kPlatformTypeX11EmbedWindowID) != Steinberg_kResultOk);
    assert(view->lpVtbl->attached(view, parent, Steinberg_kPlatformTypeX11EmbedWindowID) == Steinberg_kResultOk);
    assert(frame.resizes == 1 && frame.beforeResize.right == 300);
    assert(view->lpVtbl->attached(view, parent, Steinberg_kPlatformTypeX11EmbedWindowID) != Steinberg_kResultOk);
    check_size(view, 320, 240);
    assert(scale->lpVtbl->setContentScaleFactor(scale, 0) != Steinberg_kResultOk);
    assert(scale->lpVtbl->setContentScaleFactor(scale, 1.5f) == Steinberg_kResultOk);
    assert(frame.resizes == 2 && frame.beforeResize.right == 320);
    check_size(view, 480, 360);

    struct Steinberg_ViewRect proposed = {5, 10, 55, 60};
    assert(view->lpVtbl->checkSizeConstraint(view, &proposed) == Steinberg_kResultOk);
    assert(proposed.left == 5 && proposed.top == 10 && proposed.right == 105 && proposed.bottom == 110);
    check_size(view, 480, 360);
    assert(view->lpVtbl->onSize(view, &proposed) == Steinberg_kResultOk);
    check_size(view, 100, 100);
    assert(view->lpVtbl->onFocus(view, 1) == Steinberg_kResultOk);
    assert(view->lpVtbl->onWheel(view, 2.5f) == Steinberg_kResultOk);
    assert(view->lpVtbl->onKeyDown(view, 'A', 7, 2) == Steinberg_kResultFalse);
    assert(controller->lpVtbl->setParamNormalized(controller, 0, 0.25) == Steinberg_kResultOk);
    assert(view->lpVtbl->onKeyDown(view, 'A', 7, 2) == Steinberg_kResultOk);
    assert(view->lpVtbl->onKeyUp(view, 'B', 7, 2) == Steinberg_kResultFalse);
    assert(view->lpVtbl->onKeyUp(view, 'A', 7, 2) == Steinberg_kResultOk);

    Steinberg_Vst_IEditController* other = create_controller(factory, 0);
    Steinberg_IPlugView* otherView = other->lpVtbl->createView(other, Steinberg_Vst_ViewType_kEditor);
    HostFrame otherFrame = {{&frame_vtable}, 1, 0, {0}};
    assert(otherView->lpVtbl->setFrame(otherView, &otherFrame.iface) == Steinberg_kResultOk);
    assert(otherView->lpVtbl->attached(otherView, parent, Steinberg_kPlatformTypeX11EmbedWindowID) == Steinberg_kResultOk);

    Steinberg_Vst_IAudioProcessor* audio = NULL;
    Steinberg_Vst_IComponent* component = NULL;
    assert(controller->lpVtbl->queryInterface(controller, Steinberg_Vst_IAudioProcessor_iid, (void**)&audio) == Steinberg_kResultOk);
    assert(controller->lpVtbl->queryInterface(controller, Steinberg_Vst_IComponent_iid, (void**)&component) == Steinberg_kResultOk);
    assert(component->lpVtbl->initialize(component, NULL) == Steinberg_kResultOk);
    struct Steinberg_Vst_ProcessSetup setup = {0, 0, 64, 48000};
    assert(audio->lpVtbl->setupProcessing(audio, &setup) == Steinberg_kResultOk);
    assert(component->lpVtbl->setActive(component, 1) == Steinberg_kResultOk);
    assert(audio->lpVtbl->setProcessing(audio, 1) == Steinberg_kResultOk);
    HostHandler handler = {.iface = {&handler_vtable}, .references = 1, .audio = audio};
    assert(controller->lpVtbl->setComponentHandler(controller, &handler.iface) == Steinberg_kResultOk);
    assert(handler.references == 2);
    StateStream saved = {.stream = {&state_vtbl}};
    assert(component->lpVtbl->getState(component, &saved.stream) == Steinberg_kResultOk && saved.size > 0);
    assert(audio->lpVtbl->getLatencySamples(audio) == 96);
    assert(view->lpVtbl->onKeyDown(view, 'G', 0, 0) == Steinberg_kResultOk);
    assert(handler.steps == 4 && handler.restarts == 1 && handler.latency == 32);
    assert(controller->lpVtbl->getParamNormalized(controller, 0) == 0.75);
    assert(other->lpVtbl->getParamNormalized(other, 0) == 1.0);
    saved.cursor = 0;
    assert(component->lpVtbl->setState(component, &saved.stream) == Steinberg_kResultOk);
    assert(handler.steps == 4 && handler.restarts == 2 && handler.latency == 96);
    assert(view->lpVtbl->onKeyDown(view, 'A', 7, 2) == Steinberg_kResultOk);
    assert(view->lpVtbl->onKeyUp(view, 'A', 7, 2) == Steinberg_kResultOk);
    assert(controller->lpVtbl->setComponentHandler(controller, NULL) == Steinberg_kResultOk);
    assert(handler.references == 1);

    // Apply the controller's saved UI state to a different instance. Its
    // processor value is untouched, and an attached view sees the UI change.
    StateStream editorSaved = {.stream = {&state_vtbl}};
    assert(view->lpVtbl->onKeyDown(view, 'T', 0, 0) == Steinberg_kResultOk);
    assert(controller->lpVtbl->getState(controller, &editorSaved.stream) == Steinberg_kResultOk);
    assert(editorSaved.size == 1 && editorSaved.data[0] == 1);
    assert(otherView->lpVtbl->onKeyDown(otherView, 'C', 0, 0) == Steinberg_kResultFalse);
    editorSaved.cursor = 0;
    assert(other->lpVtbl->setState(other, &editorSaved.stream) == Steinberg_kResultOk);
    assert(otherView->lpVtbl->onKeyDown(otherView, 'C', 0, 0) == Steinberg_kResultOk);
    assert(other->lpVtbl->getParamNormalized(other, 0) == 1.0);

    assert(otherView->lpVtbl->removed(otherView) == Steinberg_kResultOk);
    assert(otherView->lpVtbl->release(otherView) == 0 && otherFrame.references == 1);
    assert(other->lpVtbl->release(other) == 0);

    Steinberg_IPlugView* reopened = controller->lpVtbl->createView(controller, Steinberg_Vst_ViewType_kEditor);
    assert(reopened && reopened != view);
    check_size(reopened, 300, 200);
    assert(view->lpVtbl->removed(view) == Steinberg_kResultOk);
    assert(view->lpVtbl->onKeyDown(view, 'A', 7, 2) == Steinberg_kResultFalse);

    float leftIn = 0.25f, rightIn = 0.5f, leftOut = 0, rightOut = 0;
    float* in[] = {&leftIn, &rightIn};
    float* out[] = {&leftOut, &rightOut};
    struct Steinberg_Vst_AudioBusBuffers input = {.numChannels = 2, .Steinberg_Vst_AudioBusBuffers_channelBuffers32 = in};
    struct Steinberg_Vst_AudioBusBuffers output = {.numChannels = 2, .Steinberg_Vst_AudioBusBuffers_channelBuffers32 = out};
    struct Steinberg_Vst_ProcessData block = {.numSamples = 1, .numInputs = 1, .numOutputs = 1, .inputs = &input, .outputs = &output};
    assert(audio->lpVtbl->process(audio, &block) == Steinberg_kResultOk);
    assert(leftIn == leftOut && rightIn == rightOut);
    assert(audio->lpVtbl->setProcessing(audio, 0) == Steinberg_kResultOk);
    assert(component->lpVtbl->setActive(component, 0) == Steinberg_kResultOk);
    audio->lpVtbl->release(audio);
    assert(component->lpVtbl->terminate(component) == Steinberg_kResultOk);
    component->lpVtbl->release(component);

    assert(view->lpVtbl->attached(view, parent, Steinberg_kPlatformTypeX11EmbedWindowID) == Steinberg_kResultOk);
    assert(frame.resizes == 3);
    check_size(view, 320, 240);
    assert(view->lpVtbl->setFrame(view, NULL) == Steinberg_kResultOk);
    assert(frame.references == 1);
    assert(view->lpVtbl->setFrame(view, &frame.iface) == Steinberg_kResultOk);

    // The component and frame remain live while the scale interface holds the view.
    assert(controller->lpVtbl->release(controller) == 2);
    assert(reopened->lpVtbl->release(reopened) == 0);
    assert(view->lpVtbl->release(view) == 1);
    assert(frame.references == 2);
    assert(scale->lpVtbl->release(scale) == 0);
    assert(frame.references == 1);

    Steinberg_Vst_IEditController* generic = create_controller(factory, 1);
    assert(generic->lpVtbl->createView(generic, Steinberg_Vst_ViewType_kEditor) == NULL);
    check_programs(factory, generic);
    generic->lpVtbl->release(generic);
    factory->lpVtbl->release(factory);
    return 0;
}
