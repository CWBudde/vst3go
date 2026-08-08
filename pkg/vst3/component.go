package vst3

// #include "../../include/vst3/vst3_c_api.h"
//
// // The header declares the interface IDs as "static const" arrays, which cgo
// // cannot reference directly from Go (it emits an extern symbol that never
// // links). These accessors keep the values inside C where they are defined.
// static inline const Steinberg_int8* uid_IComponent(void)      { return Steinberg_Vst_IComponent_iid; }
// static inline const Steinberg_int8* uid_IAudioProcessor(void) { return Steinberg_Vst_IAudioProcessor_iid; }
// static inline const Steinberg_int8* uid_IEditController(void) { return Steinberg_Vst_IEditController_iid; }
// static inline const Steinberg_int8* uid_IBStream(void)        { return Steinberg_IBStream_iid; }
// static inline const Steinberg_int8* uid_FUnknown(void)        { return Steinberg_FUnknown_iid; }
// static inline const Steinberg_int8* uid_IPluginFactory(void)  { return Steinberg_IPluginFactory_iid; }
import "C"
import "unsafe"

// Component interface IDs.
//
// These are derived from the C header rather than transcribed by hand: the
// previously hand-written literals for IComponent, IAudioProcessor and
// IEditController were all wrong, and a wrong IID makes queryInterface silently
// fail to match with no diagnostic anywhere.
var (
	IIDFUnknown        = tuidToBytes(C.uid_FUnknown())
	IIDIPluginFactory  = tuidToBytes(C.uid_IPluginFactory())
	IIDIComponent      = tuidToBytes(C.uid_IComponent())
	IIDIAudioProcessor = tuidToBytes(C.uid_IAudioProcessor())
	IIDIEditController = tuidToBytes(C.uid_IEditController())
	IIDIBStream        = tuidToBytes(C.uid_IBStream())
)

// tuidToBytes copies a C Steinberg_TUID into a Go array. The copy matters: the C
// value is const storage owned by the translation unit.
func tuidToBytes(uid *C.Steinberg_int8) [16]byte {
	return *(*[16]byte)(unsafe.Pointer(uid))
}

// IComponent represents the main plugin component interface
type IComponent interface {
	// IPluginBase methods
	Initialize(context interface{}) error
	Terminate() error

	// IComponent methods
	GetControllerClassID() [16]byte
	SetIOMode(mode int32) error
	GetBusCount(mediaType, direction int32) int32
	GetBusInfo(mediaType, direction, index int32) (*BusInfo, error)
	GetRoutingInfo(inInfo, outInfo interface{}) error
	ActivateBus(mediaType, direction, index int32, state bool) error
	SetActive(state bool) error
	SetState(state []byte) error
	GetState() ([]byte, error)
}

// IAudioProcessor represents the audio processing interface
type IAudioProcessor interface {
	SetBusArrangements(inputs, outputs []int64) error
	GetBusArrangement(direction, index int32) (int64, error)
	CanProcessSampleSize(symbolicSampleSize int32) error
	GetLatencySamples() uint32
	SetupProcessing(setup *ProcessSetup) error
	SetProcessing(state bool) error
	Process(data unsafe.Pointer) error
	GetTailSamples() uint32
}

// IEditController represents the parameter control interface
type IEditController interface {
	// IPluginBase methods
	Initialize(context interface{}) error
	Terminate() error

	// IEditController methods
	SetComponentState(state []byte) error
	SetState(state []byte) error
	GetState() ([]byte, error)
	GetParameterCount() int32
	GetParameterInfo(index int32) (*ParameterInfo, error)
	GetParamStringByValue(id uint32, value float64) (string, error)
	GetParamValueByString(id uint32, str string) (float64, error)
	NormalizedParamToPlain(id uint32, normalized float64) float64
	PlainParamToNormalized(id uint32, plain float64) float64
	GetParamNormalized(id uint32) float64
	SetParamNormalized(id uint32, value float64) error
	SetComponentHandler(handler interface{}) error
	CreateView(name string) (interface{}, error)
}
