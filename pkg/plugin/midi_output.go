package plugin

// #cgo CFLAGS: -I../../include
// #include "../../include/vst3/vst3_c_api.h"
//
// static inline void addOutputNote(struct Steinberg_Vst_IEventList* list, int32_t offset,
//                                 uint8_t channel, uint8_t pitch, uint8_t velocity, int noteOn) {
//     if (!list->lpVtbl || !list->lpVtbl->addEvent)
//         return;
//
//     struct Steinberg_Vst_Event event = {.sampleOffset = offset};
//     float level = (float)velocity / 127.0f;
//     if (noteOn) {
//         event.type = Steinberg_Vst_Event_EventTypes_kNoteOnEvent;
//         event.Steinberg_Vst_Event_noteOn = (struct Steinberg_Vst_NoteOnEvent){
//             .channel = channel, .pitch = pitch, .velocity = level, .noteId = -1,
//         };
//     } else {
//         event.type = Steinberg_Vst_Event_EventTypes_kNoteOffEvent;
//         event.Steinberg_Vst_Event_noteOff = (struct Steinberg_Vst_NoteOffEvent){
//             .channel = channel, .pitch = pitch, .velocity = level, .noteId = -1,
//         };
//     }
//     list->lpVtbl->addEvent(list, &event);
// }
import "C"

import (
	"github.com/cwbudde/vst3go/pkg/framework/process"
	"github.com/cwbudde/vst3go/pkg/midi"
)

// sendOutputNotes drains notes for the first event output bus. The host
// expects offsets within its entire block, while DSP supplies chunk offsets.
func (c *componentImpl) sendOutputNotes(ctx *process.Context, list *C.struct_Steinberg_Vst_IEventList, start int) {
	if list == nil || c.noteOutputBus == nil || !c.noteOutputBus.Active() {
		return
	}

	events := ctx.AppendOutputEvents(c.outputEvents[:0])
	for _, event := range events {
		if event.Offset < 0 || int(event.Offset) >= ctx.NumSamples() ||
			event.Note() > 127 || event.Velocity() > 127 || event.Channel > 15 {
			continue
		}
		switch event.Type {
		case midi.EventTypeNoteOn:
			C.addOutputNote(list, C.int32_t(start+int(event.Offset)), C.uint8_t(event.Channel),
				C.uint8_t(event.Note()), C.uint8_t(event.Velocity()), 1)
		case midi.EventTypeNoteOff:
			C.addOutputNote(list, C.int32_t(start+int(event.Offset)), C.uint8_t(event.Channel),
				C.uint8_t(event.Note()), C.uint8_t(event.Velocity()), 0)
		}
	}

	clear(events)
	c.outputEvents = events[:0]
	ctx.ClearOutputEvents()
}
