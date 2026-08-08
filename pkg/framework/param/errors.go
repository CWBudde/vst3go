package param

import "errors"

// Sentinel errors returned by this package. They are wrapped with %w so callers
// can classify failures with errors.Is instead of matching message text.
var (
	// ErrInvalidNumber indicates a string could not be parsed as a number.
	ErrInvalidNumber = errors.New("invalid number")
	// ErrUnknownOption indicates a string did not match any choice option.
	ErrUnknownOption = errors.New("unknown option")
	// ErrUnknownFilterType indicates a string did not match any filter type.
	ErrUnknownFilterType = errors.New("unknown filter type")
	// ErrUnknownGateType indicates a string did not match any gate type.
	ErrUnknownGateType = errors.New("unknown gate type")
	// ErrInvalidNote indicates a string could not be parsed as a note name.
	ErrInvalidNote = errors.New("invalid note")
	// ErrInvalidOnOff indicates a string was neither an on nor an off value.
	ErrInvalidOnOff = errors.New("invalid on/off value")

	// ErrMissingName indicates a parameter was built without a name.
	ErrMissingName = errors.New("parameter name must not be empty")
	// ErrInvalidRange indicates Max is not greater than Min.
	ErrInvalidRange = errors.New("parameter max must be greater than min")
	// ErrInvalidStepCount indicates the step count is negative or inconsistent
	// with the plain range. VST3 counts intervals, so a stepped parameter must
	// satisfy Max-Min == StepCount.
	ErrInvalidStepCount = errors.New("invalid step count")
	// ErrNoOptions indicates a choice parameter was created without options.
	ErrNoOptions = errors.New("choice parameter requires at least one option")

	// ErrDuplicateID indicates a parameter ID is already registered.
	ErrDuplicateID = errors.New("parameter ID already registered")
	// ErrDuplicateName indicates a parameter name is already registered.
	ErrDuplicateName = errors.New("parameter name already registered")
)
