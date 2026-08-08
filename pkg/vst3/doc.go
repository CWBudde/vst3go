// Package vst3 holds the Go bindings for Steinberg's VST 3 C API.
//
// This is the lowest layer of the repository. It is a thin, mechanical mapping
// of the C header in include/vst3 onto Go types: result codes, interface IDs,
// the process-data and bus structures, and safe wrappers over the host's audio
// buffers and IBStream. It deliberately contains no plugin logic — everything
// that decides how a plugin behaves lives in pkg/plugin and pkg/framework.
//
// # What is here
//
//   - Type aliases and result codes for the C API ([Result], [TUID],
//     [ResultOK], [Error] and its sentinels).
//   - Interface IDs read out of the C header rather than transcribed by hand
//     ([IIDIComponent], [IIDIAudioProcessor], …), so queryInterface dispatch
//     cannot silently fail on a mistyped literal.
//   - [AudioBuffer], which presents the host's channel pointer table as Go
//     slices that alias host memory without copying.
//   - [StreamWrapper], a wrapper over Steinberg_IBStream with bounded reads,
//     plus [MemoryStream], an in-process IBStream implementation used to test
//     state save/load without a host.
//
// # Untrusted input
//
// Anything arriving through an IBStream — a preset from disk, a payload from a
// host — is untrusted. Length prefixes are therefore capped ([MaxStringLength],
// [MaxStreamSize]) so a corrupt stream cannot drive an unbounded allocation.
//
// # Threading
//
// Nothing in this package is synchronized; it holds no shared mutable state of
// its own. The applicable rule is the host's: an [AudioBuffer] and the
// [ProcessData] it came from are valid only for the duration of the process
// call that delivered them, and must never be retained past it. Stream calls
// reenter the host and must not be made from the audio thread.
//
// # cgo
//
// This package imports "C" and cannot be used from _test.go files that need C
// types. Tests that must cross the boundary live in pkg/plugin or in
// test/smoke, which builds the runtime as a real loadable module and drives it
// from C.
package vst3
