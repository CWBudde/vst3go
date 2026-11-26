#include <vst3/vst3_c_api.h>
#include <stdio.h>

extern struct Steinberg_IPluginFactory* GetPluginFactory(void);

#define REQUIRE(condition) do { if (!(condition)) { \
    fprintf(stderr, "line %d: %s\n", __LINE__, #condition); return 1; \
} } while (0)

enum { frames = 8, gain_id = 0, bypass_id = 1, controller_id = 2 };

typedef struct {
    Steinberg_Vst_IEventList iface;
    const struct Steinberg_Vst_Event* events;
    Steinberg_int32 count;
} EventList;

static Steinberg_int32 SMTG_STDMETHODCALLTYPE event_count(void* self) {
    return ((EventList*)self)->count;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE get_event(void* self, Steinberg_int32 index,
                                                         struct Steinberg_Vst_Event* event) {
    const EventList* list = self;
    if (index < 0 || index >= list->count) return Steinberg_kInvalidArgument;
    *event = list->events[index];
    return Steinberg_kResultOk;
}

static Steinberg_Vst_IEventListVtbl event_vtbl = {
    .getEventCount = event_count, .getEvent = get_event,
};

typedef struct {
    Steinberg_Vst_IEventList iface;
    struct Steinberg_Vst_Event events[frames];
    Steinberg_int32 count;
} OutputEvents;

static Steinberg_tresult SMTG_STDMETHODCALLTYPE add_event(void* self, struct Steinberg_Vst_Event* event) {
    OutputEvents* output = self;
    if (output->count == frames) return Steinberg_kResultFalse;
    output->events[output->count++] = *event;
    return Steinberg_kResultOk;
}

static Steinberg_Vst_IEventListVtbl output_vtbl = {.addEvent = add_event};

static int expect_echo(const OutputEvents* output, const struct Steinberg_Vst_Event* notes, int count) {
    REQUIRE(output->count == count);
    for (int i = 0; i < count; i++) {
        const struct Steinberg_Vst_Event* observed = &output->events[i];
        REQUIRE(observed->busIndex == 0);
        REQUIRE(observed->sampleOffset == notes[i].sampleOffset);
        REQUIRE(observed->type == notes[i].type);
        if (observed->type == Steinberg_Vst_Event_EventTypes_kNoteOnEvent) {
            struct Steinberg_Vst_NoteOnEvent note = observed->Steinberg_Vst_Event_noteOn;
            REQUIRE(note.channel == 3 && note.pitch == 60 && note.velocity == 64.0f / 127.0f);
        } else {
            struct Steinberg_Vst_NoteOffEvent note = observed->Steinberg_Vst_Event_noteOff;
            REQUIRE(note.channel == 3 && note.pitch == 60 && note.velocity == 0);
        }
    }
    return 0;
}

typedef struct {
    Steinberg_int32 offset;
    Steinberg_Vst_ParamValue value;
} Point;

typedef struct {
    Steinberg_Vst_IParamValueQueue iface;
    Steinberg_Vst_ParamID id;
    const Point* points;
    Steinberg_int32 count;
} Queue;

static Steinberg_Vst_ParamID SMTG_STDMETHODCALLTYPE parameter_id(void* self) {
    return ((Queue*)self)->id;
}

static Steinberg_int32 SMTG_STDMETHODCALLTYPE point_count(void* self) {
    return ((Queue*)self)->count;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE get_point(void* self, Steinberg_int32 index,
                           Steinberg_int32* offset, Steinberg_Vst_ParamValue* value) {
    const Queue* queue = self;
    if (index < 0 || index >= queue->count) return Steinberg_kInvalidArgument;
    *offset = queue->points[index].offset;
    *value = queue->points[index].value;
    return Steinberg_kResultOk;
}

static Steinberg_Vst_IParamValueQueueVtbl queue_vtbl = {
    .getParameterId = parameter_id, .getPointCount = point_count, .getPoint = get_point,
};

typedef struct {
    Steinberg_Vst_IParameterChanges iface;
    Queue* queues;
    Steinberg_int32 count;
} ParameterChanges;

static Steinberg_int32 SMTG_STDMETHODCALLTYPE parameter_count(void* self) {
    return ((ParameterChanges*)self)->count;
}

static Steinberg_Vst_IParamValueQueue* SMTG_STDMETHODCALLTYPE get_queue(void* self, Steinberg_int32 index) {
    ParameterChanges* changes = self;
    if (index < 0 || index >= changes->count) return NULL;
    return &changes->queues[index].iface;
}

static Steinberg_Vst_IParameterChangesVtbl changes_vtbl = {
    .getParameterCount = parameter_count, .getParameterData = get_queue,
};

static struct Steinberg_Vst_Event note(Steinberg_int32 offset, int on) {
    struct Steinberg_Vst_Event event = {.sampleOffset = offset};
    if (on) {
        event.type = Steinberg_Vst_Event_EventTypes_kNoteOnEvent;
        event.Steinberg_Vst_Event_noteOn = (struct Steinberg_Vst_NoteOnEvent){.channel = 3, .pitch = 60, .velocity = 0.5f};
    } else {
        event.type = Steinberg_Vst_Event_EventTypes_kNoteOffEvent;
        event.Steinberg_Vst_Event_noteOff = (struct Steinberg_Vst_NoteOffEvent){.channel = 3, .pitch = 60};
    }
    return event;
}

static int expect_block(const char* label, float output[2][frames], const float* audio,
                        int project, int continuous, int playing, int tempo_valid) {
    for (int i = 0; i < frames; i++) {
        float timing = (float)(project + 16 * continuous);
        if (playing) timing += (float)(4096 + i * (continuous ? 17 : 1));
        if (tempo_valid) timing += 2048.0f + 8192.0f;
        if (output[0][i] != audio[i] || output[1][i] != timing) {
            fprintf(stderr, "%s frame %d: audio %.3f (want %.3f), transport %.3f (want %.3f)\n",
                    label, i, output[0][i], audio[i], output[1][i], timing);
            return 0;
        }
    }
    return 1;
}

int main(void) {
    Steinberg_IPluginFactory* factory = GetPluginFactory();
    REQUIRE(factory);
    struct Steinberg_PClassInfo ci;
    REQUIRE(factory->lpVtbl->getClassInfo(factory, 0, &ci) == Steinberg_kResultOk);
    Steinberg_Vst_IComponent* component = NULL;
    REQUIRE(factory->lpVtbl->createInstance(factory, ci.cid, Steinberg_Vst_IComponent_iid,
            (void**)&component) == Steinberg_kResultOk && component);
    REQUIRE(component->lpVtbl->initialize(component, NULL) == Steinberg_kResultOk);
    Steinberg_Vst_IAudioProcessor* audio = NULL;
    REQUIRE(component->lpVtbl->queryInterface(component, Steinberg_Vst_IAudioProcessor_iid,
            (void**)&audio) == Steinberg_kResultOk && audio);
    Steinberg_Vst_IEditController* editor = NULL;
    REQUIRE(component->lpVtbl->queryInterface(component, Steinberg_Vst_IEditController_iid,
            (void**)&editor) == Steinberg_kResultOk && editor);
    struct Steinberg_Vst_ParameterInfo bypass_info = {0};
    REQUIRE(editor->lpVtbl->getParameterInfo(editor, bypass_id, &bypass_info) == Steinberg_kResultOk);
    REQUIRE(bypass_info.id == bypass_id && (bypass_info.flags & Steinberg_Vst_ParameterInfo_ParameterFlags_kIsBypass));

    Steinberg_Vst_IMidiMapping* midi = NULL;
    REQUIRE(editor->lpVtbl->queryInterface(editor, Steinberg_Vst_IMidiMapping_iid,
            (void**)&midi) == Steinberg_kResultOk && midi);
    Steinberg_Vst_ParamID mapped = 77;
    REQUIRE(midi->lpVtbl->getMidiControllerAssignment(midi, 1, 3, 70, &mapped) == Steinberg_kResultFalse);
    REQUIRE(midi->lpVtbl->getMidiControllerAssignment(midi, 0, 0, 70, &mapped) == Steinberg_kResultFalse);
    REQUIRE(midi->lpVtbl->getMidiControllerAssignment(midi, 0, 3, 71, &mapped) == Steinberg_kResultFalse);
    REQUIRE(mapped == 77);
    REQUIRE(midi->lpVtbl->getMidiControllerAssignment(midi, 0, 3, 70, NULL) == Steinberg_kInvalidArgument);
    REQUIRE(midi->lpVtbl->getMidiControllerAssignment(midi, 0, 3, 70, &mapped) == Steinberg_kResultOk);
    REQUIRE(mapped == controller_id);
    struct Steinberg_Vst_ParameterInfo mapped_info = {0};
    REQUIRE(editor->lpVtbl->getParameterInfo(editor, controller_id, &mapped_info) == Steinberg_kResultOk);
    REQUIRE(mapped_info.id == mapped && (mapped_info.flags & Steinberg_Vst_ParameterInfo_ParameterFlags_kIsHidden));
    midi->lpVtbl->release(midi);

    REQUIRE(component->lpVtbl->getBusCount(component, Steinberg_Vst_MediaTypes_kEvent, Steinberg_Vst_BusDirections_kInput) == 1);
    REQUIRE(component->lpVtbl->activateBus(component, Steinberg_Vst_MediaTypes_kEvent, Steinberg_Vst_BusDirections_kInput, 0, 1) == Steinberg_kResultOk);
    REQUIRE(component->lpVtbl->getBusCount(component, Steinberg_Vst_MediaTypes_kEvent, Steinberg_Vst_BusDirections_kOutput) == 1);
    REQUIRE(component->lpVtbl->activateBus(component, Steinberg_Vst_MediaTypes_kEvent, Steinberg_Vst_BusDirections_kOutput, 0, 1) == Steinberg_kResultOk);
    struct Steinberg_Vst_ProcessSetup setup = {.sampleRate = 48000, .maxSamplesPerBlock = frames};
    REQUIRE(audio->lpVtbl->setupProcessing(audio, &setup) == Steinberg_kResultOk);
    REQUIRE(component->lpVtbl->setActive(component, 1) == Steinberg_kResultOk);
    REQUIRE(audio->lpVtbl->setProcessing(audio, 1) == Steinberg_kResultOk);

    float input[2][frames] = {{1, 1, 1, 1, 1, 1, 1, 1}}, output[2][frames] = {{0}};
    float* in_channels[] = {input[0], input[1]};
    float* out_channels[] = {output[0], output[1]};
    struct Steinberg_Vst_AudioBusBuffers in_bus = {.numChannels = 2, .Steinberg_Vst_AudioBusBuffers_channelBuffers32 = in_channels};
    struct Steinberg_Vst_AudioBusBuffers out_bus = {.numChannels = 2, .Steinberg_Vst_AudioBusBuffers_channelBuffers32 = out_channels};

    struct Steinberg_Vst_Event first_notes[] = {note(0, 1), note(4, 0), note(7, 1)};
    EventList events = {{&event_vtbl}, first_notes, 3};
    OutputEvents echo = {.iface = {&output_vtbl}};
    const Point first_gain[] = {{3, 0.25}, {4, 0.5}, {6, 0.5}, {7, 0.75}};
    Queue first_queue[] = {{{&queue_vtbl}, gain_id, first_gain, 4}};
    ParameterChanges changes = {{&changes_vtbl}, first_queue, 1};
    struct Steinberg_Vst_ProcessContext context = {
        .sampleRate = 48000, .projectTimeSamples = 32, .continousTimeSamples = 64, .tempo = 120,
        .projectTimeMusic = 32.0 / 24000.0,
        .state = Steinberg_Vst_ProcessContext_StatesAndFlags_kPlaying | Steinberg_Vst_ProcessContext_StatesAndFlags_kContTimeValid |
                 Steinberg_Vst_ProcessContext_StatesAndFlags_kTempoValid | Steinberg_Vst_ProcessContext_StatesAndFlags_kProjectTimeMusicValid,
    };
    struct Steinberg_Vst_ProcessData data = {
        .numSamples = frames, .numInputs = 1, .numOutputs = 1, .inputs = &in_bus, .outputs = &out_bus,
        .inputEvents = &events.iface, .inputParameterChanges = &changes.iface, .processContext = &context,
        .outputEvents = &echo.iface,
    };
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);
    REQUIRE(expect_block("automation and note offsets", output, (float[]){1.25f, 1.25f, 1.25f, 1.25f, 0.5f, 0.5f, 0.5f, 1.75f}, 32, 64, 1, 1));
    REQUIRE(expect_echo(&echo, first_notes, 3) == 0);

    struct Steinberg_Vst_Event bypass_notes[] = {note(0, 0), note(3, 1), note(7, 0)};
    const Point gain_in_first_frame[] = {{0, 0.5}};
    const Point bypass_points[] = {{1, 0}, {2, 1}, {4, 1}, {5, 0}};
    Queue bypass_queues[] = {
        {{&queue_vtbl}, gain_id, gain_in_first_frame, 1},
        {{&queue_vtbl}, bypass_id, bypass_points, 4},
    };
    events.events = bypass_notes;
    echo.count = 0;
    changes.queues = bypass_queues;
    changes.count = 2;
    context.projectTimeSamples = 40;
    context.continousTimeSamples = 72;
    context.projectTimeMusic = 40.0 / 24000.0;
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);
    REQUIRE(expect_block("next block and bypass", output, (float[]){0.5f, 0.5f, 1, 1, 1, 1.5f, 1.5f, 0.5f}, 40, 72, 1, 1));
    REQUIRE(expect_echo(&echo, bypass_notes, 3) == 0);

    const Point controller_points[] = {{3, 0.125}, {6, 0.125}};
    const Point gain_between_controller_points[] = {{5, 0.5}, {7, 0.5}};
    Queue midi_queues[] = {
        {{&queue_vtbl}, mapped, controller_points, 2},
        {{&queue_vtbl}, gain_id, gain_between_controller_points, 2},
    };
    changes.queues = midi_queues;
    changes.count = 2;
    data.inputEvents = NULL;
    data.outputEvents = NULL;
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);
    REQUIRE(expect_block("mapped controller and repeated value", output,
                         (float[]){0.5f, 0.5f, 0.5f, 0.25f, 0.25f, 0.5f, 0.25f, 0.5f}, 40, 72, 1, 1));

    const float dry_half[] = {0.5f, 0.5f, 0.5f, 0.5f, 0.5f, 0.5f, 0.5f, 0.5f};
    data.inputEvents = NULL;
    data.outputEvents = NULL;
    data.inputParameterChanges = NULL;
    context.projectTimeSamples = 0;
    context.projectTimeMusic = 0;
    context.continousTimeSamples = 80;
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);
    REQUIRE(expect_block("playback seek", output, dry_half, 0, 80, 1, 1));

    REQUIRE(audio->lpVtbl->setProcessing(audio, 0) == Steinberg_kResultOk);
    REQUIRE(component->lpVtbl->setActive(component, 0) == Steinberg_kResultOk);
    REQUIRE(component->lpVtbl->setActive(component, 1) == Steinberg_kResultOk);
    REQUIRE(audio->lpVtbl->setProcessing(audio, 1) == Steinberg_kResultOk);
    context.state = 0;
    context.continousTimeSamples = 999; // optional field is invalid after the reset
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);
    REQUIRE(expect_block("transport reset", output, dry_half, 0, 0, 0, 0));

    struct Steinberg_Vst_Event outside_block[] = {note(-1, 1), note(frames, 1)};
    events.events = outside_block;
    events.count = 2;
    data.inputEvents = &events.iface;
    data.processContext = NULL;
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);
    REQUIRE(expect_block("absent context and out-of-block events", output, dry_half, 0, 0, 0, 0));

    REQUIRE(component->lpVtbl->activateBus(component, Steinberg_Vst_MediaTypes_kAudio, Steinberg_Vst_BusDirections_kInput, 0, 0) == Steinberg_kResultOk);
    REQUIRE(component->lpVtbl->activateBus(component, Steinberg_Vst_MediaTypes_kAudio, Steinberg_Vst_BusDirections_kOutput, 0, 0) == Steinberg_kResultOk);
    data.numInputs = data.numOutputs = 0;
    data.inputs = data.outputs = NULL;

    const Point flush_gain[] = {{0, 0.75}};
    Queue flush_queue[] = {{{&queue_vtbl}, gain_id, flush_gain, 1}};
    changes.queues = flush_queue;
    changes.count = 1;
    data.numSamples = 0;
    data.inputEvents = NULL;
    data.inputParameterChanges = &changes.iface;
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);
    REQUIRE(editor->lpVtbl->getParamNormalized(editor, gain_id) == 0.75);

    struct Steinberg_Vst_Event final_frame[] = {note(7, 1)};
    events.events = final_frame;
    events.count = 1;
    const Point event_only_gain[] = {{6, 0.75}, {7, 0.25}};
    Queue event_only_queue[] = {{{&queue_vtbl}, gain_id, event_only_gain, 2}};
    changes.queues = event_only_queue;
    changes.count = 1;
    data.numSamples = frames;
    data.inputEvents = &events.iface;
    data.inputParameterChanges = &changes.iface;
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);

    REQUIRE(component->lpVtbl->activateBus(component, Steinberg_Vst_MediaTypes_kAudio, Steinberg_Vst_BusDirections_kInput, 0, 1) == Steinberg_kResultOk);
    REQUIRE(component->lpVtbl->activateBus(component, Steinberg_Vst_MediaTypes_kAudio, Steinberg_Vst_BusDirections_kOutput, 0, 1) == Steinberg_kResultOk);
    data.numInputs = data.numOutputs = 1;
    data.inputs = &in_bus;
    data.outputs = &out_bus;
    data.inputEvents = NULL;
    data.inputParameterChanges = NULL;
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);
    REQUIRE(expect_block("event-only block retained its final note and gain", output,
                         (float[]){1.25f, 1.25f, 1.25f, 1.25f, 1.25f, 1.25f, 1.25f, 1.25f}, 0, 0, 0, 0));

    data.inputEvents = &events.iface;
    data.outputEvents = &echo.iface;
    echo.count = 0;
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);
    REQUIRE(expect_echo(&echo, final_frame, 1) == 0);
    REQUIRE(component->lpVtbl->activateBus(component, Steinberg_Vst_MediaTypes_kEvent, Steinberg_Vst_BusDirections_kOutput, 0, 0) == Steinberg_kResultOk);
    echo.count = 0;
    REQUIRE(audio->lpVtbl->process(audio, &data) == Steinberg_kResultOk);
    REQUIRE(echo.count == 0);

    REQUIRE(audio->lpVtbl->setProcessing(audio, 0) == Steinberg_kResultOk);
    REQUIRE(component->lpVtbl->setActive(component, 0) == Steinberg_kResultOk);
    audio->lpVtbl->release(audio);
    editor->lpVtbl->release(editor);
    REQUIRE(component->lpVtbl->terminate(component) == Steinberg_kResultOk);
    component->lpVtbl->release(component);
    factory->lpVtbl->release(factory);
    puts("MIDI and automation smoke passed");
    return 0;
}
