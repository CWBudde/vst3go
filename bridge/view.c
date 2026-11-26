#include "view.h"

#include <stdatomic.h>
#include <stdlib.h>
#include <string.h>

typedef struct PlugView PlugView;

typedef struct {
    Steinberg_IPlugViewContentScaleSupport iface;
    PlugView* view;
} ViewScale;

struct PlugView {
    Steinberg_IPlugView iface;
    ViewScale scale;
    _Atomic Steinberg_uint32 references;
    uintptr_t goView;
    Steinberg_Vst_IEditController* owner;
    Steinberg_IPlugFrame* frame;  // Accessed on the host UI thread.
    Steinberg_TBool scalable;
};

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE view_add_ref(void* self) {
    PlugView* view = self;
    return atomic_fetch_add(&view->references, 1) + 1;
}

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE view_release(void* self) {
    PlugView* view = self;
    Steinberg_uint32 remaining = atomic_fetch_sub(&view->references, 1) - 1;
    if (remaining == 0) {
        GoPlugViewDestroy(view->goView);
        if (view->frame) {
            view->frame->lpVtbl->release(view->frame);
        }
        view->owner->lpVtbl->release(view->owner);
        free(view);
    }
    return remaining;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_query_interface(void* self, const Steinberg_TUID iid, void** obj) {
    PlugView* view = self;
    if (!obj) {
        return Steinberg_kInvalidArgument;
    }
    *obj = NULL;
    if (!iid) {
        return Steinberg_kInvalidArgument;
    }
    if (memcmp(iid, Steinberg_FUnknown_iid, sizeof(Steinberg_TUID)) == 0 ||
        memcmp(iid, Steinberg_IPlugView_iid, sizeof(Steinberg_TUID)) == 0) {
        *obj = &view->iface;
    } else if (view->scalable && memcmp(iid, Steinberg_IPlugViewContentScaleSupport_iid, sizeof(Steinberg_TUID)) == 0) {
        *obj = &view->scale.iface;
    } else {
        return Steinberg_kNoInterface;
    }
    view_add_ref(view);
    return Steinberg_kResultOk;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_supports_platform(void* self, Steinberg_FIDString type) {
    PlugView* view = self;
    return GoPlugViewSupportsPlatform(view->goView, (char*)type);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_attach(void* self, void* parent, Steinberg_FIDString type) {
    PlugView* view = self;
    return GoPlugViewAttach(view->goView, (uintptr_t)parent, (char*)type);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_remove(void* self) {
    PlugView* view = self;
    return GoPlugViewRemove(view->goView);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_wheel(void* self, float distance) {
    PlugView* view = self;
    return GoPlugViewWheel(view->goView, distance);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_key_down(void* self, Steinberg_char16 key,
                                                             Steinberg_int16 code, Steinberg_int16 modifiers) {
    PlugView* view = self;
    return GoPlugViewKey(view->goView, key, code, modifiers, 1);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_key_up(void* self, Steinberg_char16 key,
                                                           Steinberg_int16 code, Steinberg_int16 modifiers) {
    PlugView* view = self;
    return GoPlugViewKey(view->goView, key, code, modifiers, 0);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_get_size(void* self, struct Steinberg_ViewRect* size) {
    PlugView* view = self;
    return GoPlugViewGetSize(view->goView, size);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_resize(void* self, struct Steinberg_ViewRect* size) {
    PlugView* view = self;
    return GoPlugViewResize(view->goView, size);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_focus(void* self, Steinberg_TBool focused) {
    PlugView* view = self;
    return GoPlugViewFocus(view->goView, focused);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_set_frame(void* self, Steinberg_IPlugFrame* frame) {
    PlugView* view = self;
    Steinberg_IPlugFrame* old = view->frame;
    if (old == frame) {
        return Steinberg_kResultOk;
    }
    if (frame) {
        frame->lpVtbl->addRef(frame);
    }
    view->frame = frame;
    if (old) {
        old->lpVtbl->release(old);
    }
    return Steinberg_kResultOk;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_can_resize(void* self) {
    PlugView* view = self;
    return GoPlugViewCanResize(view->goView);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_constrain_size(void* self, struct Steinberg_ViewRect* size) {
    PlugView* view = self;
    return GoPlugViewConstrainSize(view->goView, size);
}

static Steinberg_IPlugViewVtbl view_vtable = {
    .queryInterface = view_query_interface,
    .addRef = view_add_ref,
    .release = view_release,
    .isPlatformTypeSupported = view_supports_platform,
    .attached = view_attach,
    .removed = view_remove,
    .onWheel = view_wheel,
    .onKeyDown = view_key_down,
    .onKeyUp = view_key_up,
    .getSize = view_get_size,
    .onSize = view_resize,
    .onFocus = view_focus,
    .setFrame = view_set_frame,
    .canResize = view_can_resize,
    .checkSizeConstraint = view_constrain_size,
};

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_scale_query_interface(void* self, const Steinberg_TUID iid, void** obj) {
    return view_query_interface(((ViewScale*)self)->view, iid, obj);
}

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE view_scale_add_ref(void* self) {
    return view_add_ref(((ViewScale*)self)->view);
}

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE view_scale_release(void* self) {
    return view_release(((ViewScale*)self)->view);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE view_scale_set(void* self, float factor) {
    return GoPlugViewScale(((ViewScale*)self)->view->goView, factor);
}

static Steinberg_IPlugViewContentScaleSupportVtbl view_scale_vtable = {
    .queryInterface = view_scale_query_interface,
    .addRef = view_scale_add_ref,
    .release = view_scale_release,
    .setContentScaleFactor = view_scale_set,
};

Steinberg_IPlugView* createPlugView(Steinberg_Vst_IEditController* owner, uintptr_t component) {
    PlugView* view = calloc(1, sizeof(*view));
    if (!view) {
        return NULL;
    }
    view->goView = GoEditControllerCreateView(component, view, &view->scalable);
    if (!view->goView) {
        free(view);
        return NULL;
    }
    view->iface.lpVtbl = &view_vtable;
    view->scale.iface.lpVtbl = &view_scale_vtable;
    view->scale.view = view;
    view->owner = owner;
    atomic_init(&view->references, 1);
    owner->lpVtbl->addRef(owner);
    return &view->iface;
}

Steinberg_tresult requestPlugViewResize(void* self, struct Steinberg_ViewRect* size) {
    PlugView* view = self;
    Steinberg_IPlugFrame* frame = view->frame;
    if (!frame) {
        return Steinberg_kResultFalse;
    }
    // resizeView may reenter the plugin and replace the frame.
    frame->lpVtbl->addRef(frame);
    view_add_ref(view);
    Steinberg_tresult result = frame->lpVtbl->resizeView(frame, &view->iface, size);
    frame->lpVtbl->release(frame);
    view_release(view);
    return result;
}

typedef struct {
    Steinberg_Linux_ITimerHandler iface;
    Steinberg_Linux_IRunLoop* loop;
    _Atomic Steinberg_uint32 references;
    uintptr_t id;  // Accessed on the host UI thread.
} PlugViewTimer;

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE timer_add_ref(void* self) {
    PlugViewTimer* timer = self;
    return atomic_fetch_add(&timer->references, 1) + 1;
}

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE timer_release(void* self) {
    PlugViewTimer* timer = self;
    Steinberg_uint32 references = atomic_fetch_sub(&timer->references, 1) - 1;
    if (references == 0) {
        free(timer);
    }
    return references;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE timer_query(void* self, const Steinberg_TUID iid, void** obj) {
    if (!obj || !iid) {
        return Steinberg_kInvalidArgument;
    }
    *obj = NULL;
    if (memcmp(iid, Steinberg_FUnknown_iid, sizeof(Steinberg_TUID)) != 0 &&
        memcmp(iid, Steinberg_Linux_ITimerHandler_iid, sizeof(Steinberg_TUID)) != 0) {
        return Steinberg_kNoInterface;
    }
    *obj = self;
    timer_add_ref(self);
    return Steinberg_kResultOk;
}

static void SMTG_STDMETHODCALLTYPE timer_tick(void* self) {
    PlugViewTimer* timer = self;
    if (timer->id) {
        // A callback can stop its own timer or cause the host to close the view.
        timer_add_ref(timer);
        GoPlugViewTimerTick(timer->id);
        timer_release(timer);
    }
}

static Steinberg_Linux_ITimerHandlerVtbl timer_vtable = {
    .queryInterface = timer_query,
    .addRef = timer_add_ref,
    .release = timer_release,
    .onTimer = timer_tick,
};

void* startPlugViewTimer(void* self, uintptr_t id, Steinberg_Linux_TimerInterval interval) {
    PlugView* view = self;
    Steinberg_IPlugFrame* frame = view->frame;
    Steinberg_Linux_IRunLoop* loop = NULL;
    if (!frame || !frame->lpVtbl->queryInterface ||
        frame->lpVtbl->queryInterface(frame, Steinberg_Linux_IRunLoop_iid, (void**)&loop) != Steinberg_kResultOk || !loop) {
        return NULL;
    }

    PlugViewTimer* timer = calloc(1, sizeof(*timer));
    if (timer) {
        timer->iface.lpVtbl = &timer_vtable;
        timer->loop = loop;
        timer->id = id;
        atomic_init(&timer->references, 1);
        if (loop->lpVtbl->registerTimer(loop, &timer->iface, interval) == Steinberg_kResultOk) {
            return timer;
        }
        timer->id = 0;
        timer_release(timer);
    }
    loop->lpVtbl->release(loop);
    return NULL;
}

void stopPlugViewTimer(void* self) {
    PlugViewTimer* timer = self;
    Steinberg_Linux_IRunLoop* loop = timer->loop;
    timer->id = 0;
    loop->lpVtbl->unregisterTimer(loop, &timer->iface);
    loop->lpVtbl->release(loop);
    timer_release(timer);
}
