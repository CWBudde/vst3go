package param

import (
	"errors"
	"math"
	"testing"
)

func TestBuilderDefaultIsOrderIndependent(t *testing.T) {
	tests := []struct {
		name    string
		build   func() *Parameter
		wantDef float64
	}{
		{
			name:    "range then default",
			build:   func() *Parameter { return New(1, "P").Range(0, 10).Default(2).Build() },
			wantDef: 0.2,
		},
		{
			name:    "default then range",
			build:   func() *Parameter { return New(1, "P").Default(2).Range(0, 10).Build() },
			wantDef: 0.2,
		},
		{
			name:    "default then toggle",
			build:   func() *Parameter { return New(1, "P").Default(1).Toggle().Build() },
			wantDef: 1,
		},
		{
			name:    "toggle then default",
			build:   func() *Parameter { return New(1, "P").Toggle().Default(1).Build() },
			wantDef: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.build()
			if p.DefaultValue != tt.wantDef {
				t.Errorf("DefaultValue = %v, want %v", p.DefaultValue, tt.wantDef)
			}
			if p.GetNormalized() != tt.wantDef {
				t.Errorf("initial value = %v, want %v", p.GetNormalized(), tt.wantDef)
			}
		})
	}
}

func TestBuilderFlags(t *testing.T) {
	t.Run("ReadOnlyAndHiddenCombine", func(t *testing.T) {
		p := New(1, "P").ReadOnly().Hidden().Build()
		if p.Flags&IsReadOnly == 0 || p.Flags&IsHidden == 0 {
			t.Errorf("Flags = %#x, want IsReadOnly|IsHidden", p.Flags)
		}
		if p.Flags&CanAutomate != 0 {
			t.Error("read-only parameter must not be automatable")
		}
	})

	t.Run("FlagsReplaces", func(t *testing.T) {
		p := New(1, "P").Hidden().Flags(IsList).Build()
		if p.Flags != IsList {
			t.Errorf("Flags = %#x, want %#x (Flags replaces)", p.Flags, IsList)
		}
	})

	t.Run("AddAndRemove", func(t *testing.T) {
		p := New(1, "P").AddFlags(IsList).RemoveFlags(CanAutomate).Build()
		if p.Flags != IsList {
			t.Errorf("Flags = %#x, want %#x", p.Flags, IsList)
		}
	})
}

func TestBuilderValidation(t *testing.T) {
	tests := []struct {
		name    string
		builder *Builder
		wantErr error
	}{
		{"valid", New(1, "P").Range(0, 10).Default(5), nil},
		{"empty name", New(1, ""), ErrMissingName},
		{"inverted range", New(1, "P").Range(10, 0), ErrInvalidRange},
		{"empty range", New(1, "P").Range(1, 1), ErrInvalidRange},
		{"negative steps", New(1, "P").Steps(-2), ErrInvalidStepCount},
		{"steps do not span range", New(1, "P").Range(0, 10).Steps(3), ErrInvalidStepCount},
		{"toggle is valid", New(1, "P").Toggle(), nil},
		{"non-finite default", New(1, "P").Default(math.NaN()), ErrInvalidNumber},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := tt.builder.TryBuild()
			if p == nil {
				t.Fatal("TryBuild must return the parameter even when invalid")
			}
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("TryBuild() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("TryBuild() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
