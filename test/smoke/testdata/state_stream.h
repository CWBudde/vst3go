#ifndef VST3GO_TEST_STATE_STREAM_H
#define VST3GO_TEST_STATE_STREAM_H

#include <vst3/vst3_c_api.h>
#include <string.h>

typedef struct {
    struct Steinberg_IBStream stream;
    unsigned char data[128];
    Steinberg_int32 size;
    Steinberg_int32 cursor;
} StateStream;

static Steinberg_tresult SMTG_STDMETHODCALLTYPE state_read(
    void* self, void* buffer, Steinberg_int32 count, Steinberg_int32* read) {
    StateStream* s = self;
    if (count < 0) return Steinberg_kInvalidArgument;
    Steinberg_int32 remaining = s->size - s->cursor;
    *read = count < remaining ? count : remaining;
    memcpy(buffer, s->data + s->cursor, *read);
    s->cursor += *read;
    return Steinberg_kResultOk;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE state_write(
    void* self, void* buffer, Steinberg_int32 count, Steinberg_int32* written) {
    StateStream* s = self;
    if (count < 0 || count > (Steinberg_int32)sizeof(s->data) - s->cursor) return Steinberg_kResultFalse;
    memcpy(s->data + s->cursor, buffer, count);
    s->cursor += count;
    if (s->size < s->cursor) s->size = s->cursor;
    *written = count;
    return Steinberg_kResultOk;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE state_tell(void* self, Steinberg_int64* pos) {
    (void)self;
    (void)pos;
    return Steinberg_kNotImplemented;  // exercise the non-seekable stream path
}

static struct Steinberg_IBStreamVtbl state_vtbl = {
    .read = state_read, .write = state_write, .tell = state_tell,
};

#endif
