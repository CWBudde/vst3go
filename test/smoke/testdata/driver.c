#include <vst3/vst3_c_api.h>
#include <stdio.h>
#include <string.h>

extern struct Steinberg_IPluginFactory* GetPluginFactory(void);

#define CHECK(cond, msg) do { if (!(cond)) { printf("FAIL: %s\n", msg); fails++; } else { printf("ok:   %s\n", msg); } } while(0)

#ifdef MULTI_CLASS
enum { class_count = 2 };

typedef struct {
    struct Steinberg_IBStream stream;
    unsigned char data[128];
    Steinberg_int32 size;
} StateStream;

static Steinberg_tresult SMTG_STDMETHODCALLTYPE state_read(
    void* self, void* buffer, Steinberg_int32 count, Steinberg_int32* read) {
    StateStream* s = self;
    *read = count < s->size ? count : s->size;
    memcpy(buffer, s->data, *read);
    return Steinberg_kResultOk;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE state_write(
    void* self, void* buffer, Steinberg_int32 count, Steinberg_int32* written) {
    StateStream* s = self;
    if (count < 0 || count > (Steinberg_int32)sizeof(s->data)) return Steinberg_kResultFalse;
    memcpy(s->data, buffer, count);
    *written = s->size = count;
    return Steinberg_kResultOk;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE state_tell(void* self, Steinberg_int64* pos) {
    return Steinberg_kNotImplemented;  // one-shot non-seekable parameter state
}

static struct Steinberg_IBStreamVtbl state_vtbl = {
    .read = state_read, .write = state_write, .tell = state_tell,
};

static int check_multiple_classes(struct Steinberg_IPluginFactory* f) {
    int fails = 0;
    struct Steinberg_PClassInfo infos[2] = {0};
    const char* expected_names[] = {"VST3Go Smoke Test", "VST3Go Smoke Test Auxiliary"};
    for (int i = 0; i < 2; i++) {
        CHECK(f->lpVtbl->getClassInfo(f, i, &infos[i]) == Steinberg_kResultOk,
              "class is discoverable");
        CHECK(strcmp(infos[i].name, expected_names[i]) == 0, "class in registration order");
    }
    CHECK(memcmp(infos[0].cid, infos[1].cid, sizeof(Steinberg_TUID)) != 0, "distinct CIDs");
    struct Steinberg_PClassInfo invalid;
    CHECK(f->lpVtbl->getClassInfo(f, -1, &invalid) != Steinberg_kResultOk, "negative class index rejected");
    CHECK(f->lpVtbl->getClassInfo(f, 2, &invalid) != Steinberg_kResultOk, "upper class index rejected");

    struct Steinberg_Vst_IEditController* editors[3] = {0};
    double defaults[] = {1.0, 1.0, 0.25};
    for (int i = 0; i < 3; i++) {
        Steinberg_tresult r = f->lpVtbl->createInstance(f, infos[i == 2].cid,
            Steinberg_Vst_IEditController_iid, (void**)&editors[i]);
        CHECK(r == Steinberg_kResultOk && editors[i] != NULL, "CID creates live instance");
        if (editors[i]) {
            CHECK(editors[i]->lpVtbl->getParamNormalized(editors[i], 0) == defaults[i],
                  "instance receives requested processor's default");
        }
    }

    if (editors[0] && editors[1] && editors[2]) {
        StateStream saved = {.stream = {&state_vtbl}};
        struct Steinberg_Vst_IEditController* e = editors[0];
        CHECK(e->lpVtbl->setParamNormalized(e, 0, 0.5) == Steinberg_kResultOk, "edit first instance");
        CHECK(e->lpVtbl->getState(e, &saved.stream) == Steinberg_kResultOk && saved.size > 0,
              "serialize first instance state");
        e->lpVtbl->setParamNormalized(e, 0, 0.75);
        CHECK(e->lpVtbl->setState(e, &saved.stream) == Steinberg_kResultOk, "restore first instance state");
        CHECK(e->lpVtbl->getParamNormalized(e, 0) == 0.5, "first instance value restored");
        for (int i = 1; i < 3; i++) {
            e = editors[i];
            CHECK(e->lpVtbl->getParamNormalized(e, 0) == defaults[i], "other instance retains its own state");
        }
    }
    for (int i = 0; i < 3; i++) {
        if (editors[i]) editors[i]->lpVtbl->release(editors[i]);
    }
    return fails;
}
#else
enum { class_count = 1 };
#endif

int main(void) {
    int fails = 0;
    struct Steinberg_IPluginFactory* f = GetPluginFactory();
    CHECK(f != NULL, "GetPluginFactory returns non-NULL");

    struct Steinberg_PFactoryInfo fi;
    memset(&fi, 0xAA, sizeof(fi));  // poison: catches unbounded/absent writes
    f->lpVtbl->getFactoryInfo(f, &fi);
    printf("      vendor=\"%s\"\n", fi.vendor);
    CHECK(strlen(fi.vendor) < 64, "vendor stays within char[64]");

    Steinberg_int32 n = f->lpVtbl->countClasses(f);
    CHECK(n == class_count, "countClasses matches registration");
#ifdef MULTI_CLASS
    fails += check_multiple_classes(f);
#endif

    struct Steinberg_PClassInfo ci;
    memset(&ci, 0xAA, sizeof(ci));
    f->lpVtbl->getClassInfo(f, 0, &ci);
    printf("      name=\"%s\" category=\"%s\"\n", ci.name, ci.category);
    CHECK(strlen(ci.name) < 64 && strlen(ci.category) < 32, "class strings bounded");

    // createInstance for IComponent
    void* comp = NULL;
    Steinberg_tresult r = f->lpVtbl->createInstance(f, ci.cid, Steinberg_Vst_IComponent_iid, &comp);
    CHECK(r == Steinberg_kResultOk && comp != NULL, "createInstance(IComponent)");

    if (comp) {
        struct Steinberg_Vst_IComponent* c = (struct Steinberg_Vst_IComponent*)comp;
        CHECK(c->lpVtbl->getBusCount(c, 0, 0) == 1, "getBusCount(audio,in) == 1");

        // finding 1.6: must NOT claim a separate controller class
        Steinberg_TUID cid;
        Steinberg_tresult gr = c->lpVtbl->getControllerClassId(c, cid);
        CHECK(gr != Steinberg_kResultOk, "getControllerClassId reports not-implemented");

        // finding 1.5: IEditController must be reachable and be the CONTROLLER vtable
        void* ec = NULL;
        Steinberg_tresult qr = c->lpVtbl->queryInterface(c, Steinberg_Vst_IEditController_iid, &ec);
        CHECK(qr == Steinberg_kResultOk && ec != NULL, "QI(IEditController) succeeds");
        if (ec) {
            struct Steinberg_Vst_IEditController* e = (struct Steinberg_Vst_IEditController*)ec;
            Steinberg_int32 pc = e->lpVtbl->getParameterCount(e);
            printf("      getParameterCount=%d\n", pc);
            CHECK(pc == 1, "getParameterCount == 1 (not a mis-dispatched slot)");
            e->lpVtbl->release(e);
        }
        c->lpVtbl->release(c);
    }

    // finding 1.5: creating directly as IEditController must give the controller vtable
    void* ec2 = NULL;
    r = f->lpVtbl->createInstance(f, ci.cid, Steinberg_Vst_IEditController_iid, &ec2);
    CHECK(r == Steinberg_kResultOk && ec2 != NULL, "createInstance(IEditController)");
    if (ec2) {
        struct Steinberg_Vst_IEditController* e = (struct Steinberg_Vst_IEditController*)ec2;
        Steinberg_int32 pc = e->lpVtbl->getParameterCount(e);
        printf("      getParameterCount via createInstance=%d\n", pc);
        CHECK(pc == 1, "createInstance(IEditController) returns controller vtable");
        e->lpVtbl->release(e);
    }

    // unknown CID must be rejected
    Steinberg_TUID bogus; memset(bogus, 0x11, 16);
    void* bad = (void*)0x1;
    r = f->lpVtbl->createInstance(f, bogus, Steinberg_Vst_IComponent_iid, &bad);
    CHECK(r != Steinberg_kResultOk && bad == NULL, "unknown CID rejected, obj nulled");

    // finding 1.4: release to zero, then GetPluginFactory must return a fresh object
    f->lpVtbl->release(f);
    struct Steinberg_IPluginFactory* f2 = GetPluginFactory();
    CHECK(f2 != NULL, "GetPluginFactory after full release returns non-NULL");
    Steinberg_int32 n2 = f2->lpVtbl->countClasses(f2);
    CHECK(n2 == class_count, "reborn factory is usable (no use-after-free)");

    // finding 1.1: two input buses (e.g. main + sidechain) must not panic
    {
        void* comp2 = NULL;
        f2->lpVtbl->createInstance(f2, ci.cid, Steinberg_Vst_IComponent_iid, &comp2);
        struct Steinberg_Vst_IAudioProcessor* ap = NULL;
        ((struct Steinberg_FUnknown*)comp2)->lpVtbl->queryInterface(
            comp2, Steinberg_Vst_IAudioProcessor_iid, (void**)&ap);
        CHECK(ap != NULL, "QI(IAudioProcessor) succeeds");

        struct Steinberg_Vst_ProcessSetup setup = {0, 0, 512, 48000.0};
        ap->lpVtbl->setupProcessing(ap, &setup);
        ap->lpVtbl->setProcessing(ap, 1);

        static float inA[2][512], inB[2][512], outA[2][512];
        float *pinA[2] = {inA[0], inA[1]};
        float *pinB[2] = {inB[0], inB[1]};
        float *poutA[2] = {outA[0], outA[1]};
        for (int i = 0; i < 512; i++) { inA[0][i] = 0.25f; inA[1][i] = 0.5f; }

        struct Steinberg_Vst_AudioBusBuffers ins[2] = {0};
        ins[0].numChannels = 2; ins[0].Steinberg_Vst_AudioBusBuffers_channelBuffers32 = pinA;
        ins[1].numChannels = 2; ins[1].Steinberg_Vst_AudioBusBuffers_channelBuffers32 = pinB;
        struct Steinberg_Vst_AudioBusBuffers outs[1] = {0};
        outs[0].numChannels = 2; outs[0].Steinberg_Vst_AudioBusBuffers_channelBuffers32 = poutA;

        struct Steinberg_Vst_ProcessData pd = {0};
        pd.numSamples = 512;
        pd.symbolicSampleSize = 0;
        pd.numInputs = 2;    // <-- the case that used to panic
        pd.numOutputs = 1;
        pd.inputs = ins;
        pd.outputs = outs;

        Steinberg_tresult pr = ap->lpVtbl->process(ap, &pd);
        CHECK(pr == Steinberg_kResultOk, "process() with 2 input buses returns kResultOk");
        CHECK(outA[0][0] == 0.25f && outA[1][100] == 0.5f,
              "process() with 2 input buses actually produced audio");

        ap->lpVtbl->release(ap);
        ((struct Steinberg_FUnknown*)comp2)->lpVtbl->release(comp2);
    }

    f2->lpVtbl->release(f2);

    printf("\n%s (%d failures)\n", fails ? "FAILED" : "ALL PASSED", fails);
    return fails != 0;
}
