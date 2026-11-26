package plugin

import (
	"io"

	"github.com/cwbudde/vst3go/pkg/vst3"
)

// EditorPlatform identifies the native parent supplied by the host.
type EditorPlatform string

const (
	// EditorHWND is a Windows window handle.
	EditorHWND EditorPlatform = "HWND"
	// EditorNSView is an AppKit NSView address.
	EditorNSView EditorPlatform = "NSView"
	// EditorX11EmbedWindowID is an X11 Window ID supporting XEmbed.
	EditorX11EmbedWindowID EditorPlatform = "X11EmbedWindowID"
)

// EditorSize describes the view dimensions in the units VST3 uses for its
// platform, excluding the host's window decorations.
type EditorSize struct {
	Width, Height int32
}

// EditorKey carries the host's UTF-16 code unit, VST3 virtual key code and
// modifier mask. A virtual key can have a zero character.
type EditorKey struct {
	Character uint16
	Code      int16
	Modifiers int16
}

// EditorContext is the view's connection to its host and processor instance.
// Use it on the host UI thread while the view is alive.
type EditorContext struct {
	Parameters EditorParameters
	resize     func(EditorSize) error
}

// RequestResize asks the host to resize the view. The host may call Resize
// synchronously before this returns. The native child should only be resized
// in Resize. This returns an error if the host frame is unavailable or rejects
// the change.
func (c EditorContext) RequestResize(size EditorSize) error {
	if c.resize == nil {
		return vst3.ErrNotImplemented
	}
	return c.resize(size)
}

// EditorProvider is an optional Processor extension. CreateEditor returns a
// fresh view on each request, or nil to use the host's generic editor.
type EditorProvider interface {
	CreateEditor(EditorContext) EditorView
}

// EditorStateProvider persists UI state independently of processor state.
// The host calls these methods on its control thread, including when no view
// is open. Keep the state on the processor instance so a subsequent view can
// restore it. The provider owns the format and versioning of its payload.
type EditorStateProvider interface {
	SaveEditorState(io.Writer) error
	LoadEditorState(io.Reader) error
}

// EditorView is one instance of a plugin's native editor. The host calls its
// methods on the UI thread. The view creates its native child in Attach and
// owns the child until Remove. The host owns the parent throughout.
type EditorView interface {
	SupportsPlatform(EditorPlatform) bool
	// Attach receives the native parent handle as an integer. For X11 this
	// is the X Window ID. On error, release any resources allocated for this
	// attachment.
	Attach(EditorPlatform, uintptr) error
	// Remove releases the native child. The host can attach this view again.
	Remove()
	// InitialSize returns the dimensions before the first attachment.
	InitialSize() EditorSize
	// Resize applies the host's new dimensions. It can run before Attach or
	// within a RequestResize call made by the view.
	Resize(EditorSize) error
}

// ResizableEditor opts into host-driven resizing. ConstrainSize validates or
// adjusts a proposed size without applying it.
type ResizableEditor interface {
	ConstrainSize(EditorSize) EditorSize
}

// EditorInput receives host input for an attached editor. Return true for
// events consumed by the view so the host can handle unconsumed events.
type EditorInput interface {
	OnFocus(focused bool)
	OnKey(key EditorKey, down bool) bool
	OnWheel(distance float32) bool
}

// ScalableEditor opts into VST3 content-scale notifications. Apply the factor
// to the view's content and call RequestResize if the dimensions need to
// change. The host can set the factor before attaching the view.
type ScalableEditor interface {
	SetScaleFactor(factor float32) error
}
