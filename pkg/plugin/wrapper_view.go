package plugin

// #cgo CFLAGS: -I../../include
// #include "../../bridge/view.h"
// static inline Steinberg_tresult editor_invalid_argument(void) { return Steinberg_kInvalidArgument; }
import "C"

import (
	"fmt"
	"math"
	"runtime/cgo"
	"time"
	"unsafe"

	"github.com/cwbudde/vst3go/pkg/vst3"
)

type editorAttachment uint8

const (
	editorOK    = C.Steinberg_tresult(vst3.ResultOK)
	editorFalse = C.Steinberg_tresult(vst3.ResultFalse)
)

const (
	editorDetached editorAttachment = iota
	editorAttaching
	editorAttached
	editorRemoving
)

// editorViewBridge belongs to one IPlugView. VST3 calls the view and its frame
// on the host UI thread, including any reentrant resize from RequestResize.
type editorViewBridge struct {
	view       EditorView
	native     unsafe.Pointer
	rect       C.struct_Steinberg_ViewRect
	attachment editorAttachment
	timers     map[unsafe.Pointer]cgo.Handle
}

func editorFromHandle(id C.uintptr_t) *editorViewBridge {
	return cgo.Handle(id).Value().(*editorViewBridge)
}

//export GoEditControllerCreateView
func GoEditControllerCreateView(componentID C.uintptr_t, native unsafe.Pointer, scalable *C.Steinberg_TBool) C.uintptr_t {
	defer recoverPanic("GoEditControllerCreateView")

	wrapper := getComponent(uintptr(componentID))
	if wrapper == nil {
		return 0
	}
	component, ok := wrapper.component.(*componentImpl)
	if !ok {
		return 0
	}
	provider, ok := component.processor.(EditorProvider)
	if !ok {
		return 0
	}

	view := &editorViewBridge{}
	view.view = provider.CreateEditor(EditorContext{
		Parameters: EditorParameters{wrapper: wrapper},
		resize:     view.requestResize,
		startTimer: view.startTimer,
		markDirty:  wrapper.markEditorDirty,
	})
	if view.view == nil {
		return 0
	}
	view.rect, ok = editorRect(view.rect, view.view.InitialSize())
	if !ok {
		return 0
	}
	if _, ok := view.view.(ScalableEditor); ok {
		*scalable = 1
	}
	view.native = native
	return C.uintptr_t(cgo.NewHandle(view))
}

//export GoPlugViewDestroy
func GoPlugViewDestroy(id C.uintptr_t) {
	defer recoverPanic("GoPlugViewDestroy")
	defer cgo.Handle(id).Delete()
	view := editorFromHandle(id)
	view.native = nil
	view.remove()
}

//export GoPlugViewSupportsPlatform
func GoPlugViewSupportsPlatform(id C.uintptr_t, kind *C.char) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewSupportsPlatform", &result)
	return editorResult(kind != nil && editorFromHandle(id).view.SupportsPlatform(EditorPlatform(C.GoString(kind))))
}

//export GoPlugViewAttach
func GoPlugViewAttach(id C.uintptr_t, parent C.uintptr_t, kind *C.char) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewAttach", &result)
	if parent == 0 || kind == nil {
		return C.editor_invalid_argument()
	}
	return editorResult(editorFromHandle(id).attach(EditorPlatform(C.GoString(kind)), uintptr(parent)))
}

func (v *editorViewBridge) attach(platform EditorPlatform, parent uintptr) bool {
	if v.attachment != editorDetached || !v.view.SupportsPlatform(platform) {
		return false
	}
	v.attachment = editorAttaching
	defer func() {
		if v.attachment == editorAttaching {
			v.stopTimers()
			v.attachment = editorDetached
		}
	}()
	if err := v.view.Attach(platform, parent); err != nil {
		return false
	}
	v.attachment = editorAttached
	return true
}

//export GoPlugViewRemove
func GoPlugViewRemove(id C.uintptr_t) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewRemove", &result)
	return editorResult(editorFromHandle(id).remove())
}

func (v *editorViewBridge) remove() bool {
	if v.attachment != editorAttached {
		return false
	}
	v.attachment = editorRemoving
	defer func() { v.attachment = editorDetached }()
	v.stopTimers()
	v.view.Remove()
	return true
}

//export GoPlugViewGetSize
func GoPlugViewGetSize(id C.uintptr_t, rect *C.struct_Steinberg_ViewRect) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewGetSize", &result)
	if rect == nil {
		return C.editor_invalid_argument()
	}
	*rect = editorFromHandle(id).rect
	return editorOK
}

//export GoPlugViewResize
func GoPlugViewResize(id C.uintptr_t, rect *C.struct_Steinberg_ViewRect) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewResize", &result)
	size, ok := editorSize(rect)
	if !ok || size.Width <= 0 || size.Height <= 0 {
		return C.editor_invalid_argument()
	}
	view := editorFromHandle(id)
	if err := view.view.Resize(size); err != nil {
		return editorFalse
	}
	view.rect = *rect
	return editorOK
}

//export GoPlugViewCanResize
func GoPlugViewCanResize(id C.uintptr_t) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewCanResize", &result)
	_, ok := editorFromHandle(id).view.(ResizableEditor)
	return editorResult(ok)
}

//export GoPlugViewConstrainSize
func GoPlugViewConstrainSize(id C.uintptr_t, rect *C.struct_Steinberg_ViewRect) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewConstrainSize", &result)
	size, ok := editorSize(rect)
	if !ok {
		return C.editor_invalid_argument()
	}
	view, ok := editorFromHandle(id).view.(ResizableEditor)
	if !ok {
		return editorFalse
	}
	constrained, ok := editorRect(*rect, view.ConstrainSize(size))
	if !ok {
		return editorFalse
	}
	*rect = constrained
	return editorOK
}

func (v *editorViewBridge) requestResize(size EditorSize) error {
	if v.native == nil {
		return vst3.ErrNotImplemented
	}
	rect, ok := editorRect(v.rect, size)
	if !ok {
		return vst3.ErrInvalidArgument
	}
	if result := C.requestPlugViewResize(v.native, &rect); result != editorOK {
		return fmt.Errorf("host resizeView returned %d", result)
	}
	return nil
}

func (v *editorViewBridge) startTimer(interval time.Duration, tick func()) (func(), error) {
	if tick == nil || interval <= 0 {
		return nil, vst3.ErrInvalidArgument
	}
	if v.native == nil || (v.attachment != editorAttached && v.attachment != editorAttaching) {
		return nil, vst3.ErrNotImplemented
	}
	milliseconds := interval.Milliseconds()
	if interval%time.Millisecond != 0 {
		milliseconds++
	}
	handle := cgo.NewHandle(tick)
	timer := C.startPlugViewTimer(v.native, C.uintptr_t(handle), C.Steinberg_Linux_TimerInterval(milliseconds))
	if timer == nil {
		handle.Delete()
		return nil, vst3.ErrNotImplemented
	}
	if v.timers == nil {
		v.timers = make(map[unsafe.Pointer]cgo.Handle)
	}
	v.timers[timer] = handle
	return func() { v.stopTimer(timer, handle) }, nil
}

func (v *editorViewBridge) stopTimer(timer unsafe.Pointer, handle cgo.Handle) {
	if v.timers[timer] != handle {
		return
	}
	delete(v.timers, timer)
	C.stopPlugViewTimer(timer)
	handle.Delete()
}

func (v *editorViewBridge) stopTimers() {
	for timer, handle := range v.timers {
		v.stopTimer(timer, handle)
	}
}

//export GoPlugViewTimerTick
func GoPlugViewTimerTick(id C.uintptr_t) {
	defer recoverPanic("GoPlugViewTimerTick")
	cgo.Handle(id).Value().(func())()
}

func editorSize(rect *C.struct_Steinberg_ViewRect) (EditorSize, bool) {
	if rect == nil {
		return EditorSize{}, false
	}
	width := int64(rect.right) - int64(rect.left)
	height := int64(rect.bottom) - int64(rect.top)
	if width < math.MinInt32 || width > math.MaxInt32 || height < math.MinInt32 || height > math.MaxInt32 {
		return EditorSize{}, false
	}
	return EditorSize{Width: int32(width), Height: int32(height)}, true
}

func editorRect(rect C.struct_Steinberg_ViewRect, size EditorSize) (C.struct_Steinberg_ViewRect, bool) {
	right := int64(rect.left) + int64(size.Width)
	bottom := int64(rect.top) + int64(size.Height)
	if size.Width <= 0 || size.Height <= 0 || right > math.MaxInt32 || bottom > math.MaxInt32 {
		return rect, false
	}
	rect.right, rect.bottom = C.Steinberg_int32(right), C.Steinberg_int32(bottom)
	return rect, true
}

func editorResult(ok bool) C.Steinberg_tresult {
	if ok {
		return editorOK
	}
	return editorFalse
}

func (v *editorViewBridge) input() EditorInput {
	if v.attachment != editorAttached {
		return nil
	}
	input, _ := v.view.(EditorInput)
	return input
}

//export GoPlugViewWheel
func GoPlugViewWheel(id C.uintptr_t, distance C.float) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewWheel", &result)
	input := editorFromHandle(id).input()
	return editorResult(input != nil && input.OnWheel(float32(distance)))
}

//export GoPlugViewKey
func GoPlugViewKey(id C.uintptr_t, key C.Steinberg_char16, code, modifiers C.Steinberg_int16, down C.Steinberg_TBool) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewKey", &result)
	input := editorFromHandle(id).input()
	event := EditorKey{Character: uint16(key), Code: int16(code), Modifiers: int16(modifiers)}
	return editorResult(input != nil && input.OnKey(event, down != 0))
}

//export GoPlugViewFocus
func GoPlugViewFocus(id C.uintptr_t, focused C.Steinberg_TBool) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewFocus", &result)
	input := editorFromHandle(id).input()
	if input == nil {
		return editorFalse
	}
	input.OnFocus(focused != 0)
	return editorOK
}

//export GoPlugViewScale
func GoPlugViewScale(id C.uintptr_t, factor C.float) (result C.Steinberg_tresult) {
	defer recoverPanicResult("GoPlugViewScale", &result)
	if factor <= 0 || math.IsNaN(float64(factor)) || math.IsInf(float64(factor), 0) {
		return C.editor_invalid_argument()
	}
	view, ok := editorFromHandle(id).view.(ScalableEditor)
	return editorResult(ok && view.SetScaleFactor(float32(factor)) == nil)
}
