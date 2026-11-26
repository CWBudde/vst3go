#include <vst3/vst3_c_api.h>
#include <stdio.h>
#include <string.h>

extern struct Steinberg_IPluginFactory* GetPluginFactory(void);

#define CHECK(cond, msg) do { if (!(cond)) { printf("FAIL: %s\n", msg); fails++; } else { printf("ok:   %s\n", msg); } } while(0)

#ifdef MULTI_CLASS
enum { class_count = 2 };

#include "state_stream.h"

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
        saved.cursor = 0;
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
        CHECK(c->lpVtbl->initialize(c, NULL) == Steinberg_kResultOk, "initialize component");
        CHECK(c->lpVtbl->getBusCount(c, 0, 0) == 2, "discovers main and sidechain inputs");

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
        c->lpVtbl->terminate(c);
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
        struct Steinberg_Vst_IComponent* comp2 = NULL;
        r = f2->lpVtbl->createInstance(f2, ci.cid, Steinberg_Vst_IComponent_iid, (void**)&comp2);
        CHECK(r == Steinberg_kResultOk && comp2, "create sidechain component");
        if (!comp2) return 1;
        CHECK(comp2->lpVtbl->initialize(comp2, NULL) == Steinberg_kResultOk, "initialize sidechain component");
        struct Steinberg_Vst_IAudioProcessor* ap = NULL;
        comp2->lpVtbl->queryInterface(comp2, Steinberg_Vst_IAudioProcessor_iid, (void**)&ap);
        CHECK(ap != NULL, "QI(IAudioProcessor) succeeds");
        if (!ap) return 1;

        Steinberg_Vst_SpeakerArrangement inputs[] = {Steinberg_Vst_SpeakerArr_kStereo, Steinberg_Vst_SpeakerArr_kStereo};
        Steinberg_Vst_SpeakerArrangement output = Steinberg_Vst_SpeakerArr_kStereo;
        CHECK(ap->lpVtbl->setBusArrangements(ap, inputs, 2, &output, 1) == Steinberg_kResultOk,
              "negotiate stereo main and sidechain buses");
        CHECK(comp2->lpVtbl->activateBus(comp2, 0, 0, 0, 1) == Steinberg_kResultOk, "activate main input");
        CHECK(comp2->lpVtbl->activateBus(comp2, 0, 1, 0, 1) == Steinberg_kResultOk, "activate main output");
        CHECK(comp2->lpVtbl->activateBus(comp2, 0, 0, 1, 1) == Steinberg_kResultOk, "activate sidechain");

        struct Steinberg_Vst_ProcessSetup setup = {0, 0, 512, 48000.0};
        CHECK(ap->lpVtbl->setupProcessing(ap, &setup) == Steinberg_kResultOk, "setup processing");
        CHECK(comp2->lpVtbl->setActive(comp2, 1) == Steinberg_kResultOk, "activate component");
        CHECK(ap->lpVtbl->setProcessing(ap, 1) == Steinberg_kResultOk, "start processing");

        static float inA[2][512], inB[2][512], outA[2][512];
        float *pinA[2] = {inA[0], inA[1]};
        float *pinB[2] = {inB[0], inB[1]};
        float *poutA[2] = {outA[0], outA[1]};
        inA[0][0] = 0.25f; inA[1][1] = 0.5f;
        inB[0][2] = 0.75f; inB[1][3] = 1.0f;

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
        CHECK(outA[0][0] == 0.25f && outA[1][1] == 0.5f && outA[0][1] == 0 && outA[1][0] == 0,
              "main impulses stay on their own channels");
        CHECK(outA[0][2] == 0.75f && outA[1][3] == 1.0f && outA[0][3] == 0 && outA[1][2] == 0,
              "sidechain impulses stay on their own channels");

        CHECK(comp2->lpVtbl->activateBus(comp2, 0, 0, 1, 0) == Steinberg_kResultOk, "disconnect sidechain");
        CHECK(ap->lpVtbl->process(ap, &pd) == Steinberg_kResultOk && outA[0][2] == 0 && outA[1][3] == 0,
              "inactive sidechain buffer ignored");
        pd.numInputs = 1;
        CHECK(ap->lpVtbl->process(ap, &pd) == Steinberg_kResultOk && outA[0][0] == 0.25f && outA[1][1] == 0.5f,
              "disconnected trailing bus can be omitted");
        CHECK(comp2->lpVtbl->activateBus(comp2, 0, 0, 1, 1) == Steinberg_kResultOk, "reconnect sidechain");
        pd.numInputs = 2;
        CHECK(ap->lpVtbl->process(ap, &pd) == Steinberg_kResultOk && outA[0][2] == 0.75f && outA[1][3] == 1.0f,
              "reconnected sidechain is routed to processor");

        ap->lpVtbl->setProcessing(ap, 0);
        comp2->lpVtbl->setActive(comp2, 0);
        ap->lpVtbl->release(ap);
        comp2->lpVtbl->terminate(comp2);
        comp2->lpVtbl->release(comp2);
    }

    f2->lpVtbl->release(f2);

    printf("\n%s (%d failures)\n", fails ? "FAILED" : "ALL PASSED", fails);
    return fails != 0;
}
