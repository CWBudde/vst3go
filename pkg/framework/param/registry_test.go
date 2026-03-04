package param

import "testing"

func TestRegistryAddRejectsDuplicateID(t *testing.T) {
	reg := NewRegistry()

	first := &Parameter{ID: 1, Name: "First"}
	second := &Parameter{ID: 1, Name: "Second"}

	if err := reg.Add(first); err != nil {
		t.Fatalf("Add(first) error = %v", err)
	}

	if err := reg.Add(second); err == nil {
		t.Fatal("Add(second) should fail on duplicate ID")
	}

	if got := reg.Count(); got != 1 {
		t.Fatalf("Count() = %d, want 1", got)
	}
}

func TestRegistrySafeAccessors(t *testing.T) {
	reg := NewRegistry()
	param := &Parameter{ID: 7, Name: "Gain", Min: -12, Max: 12}
	param.SetPlain(3)

	if err := reg.Add(param); err != nil {
		t.Fatalf("Add(param) error = %v", err)
	}

	if !reg.Has(7) {
		t.Fatal("Has(7) = false, want true")
	}

	if got, ok := reg.GetOK(7); !ok || got != param {
		t.Fatal("GetOK(7) did not return the registered parameter")
	}

	if got, ok := reg.GetNormalized(7); !ok || got != param.GetNormalized() {
		t.Fatalf("GetNormalized(7) = (%f, %v), want (%f, true)", got, ok, param.GetNormalized())
	}

	if got, ok := reg.GetPlain(7); !ok || got != 3 {
		t.Fatalf("GetPlain(7) = (%f, %v), want (3, true)", got, ok)
	}

	if _, ok := reg.GetOK(99); ok {
		t.Fatal("GetOK(99) should report missing parameter")
	}

	if _, ok := reg.GetNormalized(99); ok {
		t.Fatal("GetNormalized(99) should report missing parameter")
	}

	if _, ok := reg.GetPlain(99); ok {
		t.Fatal("GetPlain(99) should report missing parameter")
	}
}
