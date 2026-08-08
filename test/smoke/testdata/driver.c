#include <vst3/vst3_c_api.h>
#include <stdio.h>
#include <string.h>

extern struct Steinberg_IPluginFactory* GetPluginFactory(void);

#define CHECK(cond, msg) do { if (!(cond)) { printf("FAIL: %s\n", msg); fails++; } else { printf("ok:   %s\n", msg); } } while(0)

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
    CHECK(n == 1, "countClasses == 1");

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
    CHECK(n2 == 1, "reborn factory is usable (no use-after-free)");

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
