package plugin

// HostCall describes an observable host interaction with one plugin instance.
// Method names the VST3 interface and operation. Int contains up to three
// integral arguments in the order described by each call site. Float carries a
// scalar such as a sample rate or a normalized parameter value. Zero is used
// for omitted arguments. Records contain no host pointers or audio buffers.
type HostCall struct {
	Method string
	Int    [3]int64
	Float  float64
}

// HostCallObserver is an optional diagnostic extension. A processor that
// implements it receives instance interactions from the runtime. The host may
// call several interfaces from different threads. ObserveHostCall must return
// without blocking, allocating, writing files, or calling back into the host.
type HostCallObserver interface {
	ObserveHostCall(HostCall)
}

func (c *componentImpl) traceHostCall(method string, a, b, d int64, value float64) {
	if c.hostCallObserver != nil {
		c.hostCallObserver.ObserveHostCall(HostCall{Method: method, Int: [3]int64{a, b, d}, Float: value})
	}
}

func hostFlag(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func (w *componentWrapper) traceHostCall(method string, a, b, d int64, value float64) {
	if component, ok := w.component.(*componentImpl); ok {
		component.traceHostCall(method, a, b, d, value)
	}
}
