package backends

import "testing"

func TestSelectSoftwareAndAuto(t *testing.T) {
	for _, name := range []string{"", "auto", "software"} {
		backend, err := Select(name)
		if err != nil {
			t.Fatalf("Select(%q): %v", name, err)
		}
		if backend.Name() != "software" {
			t.Fatalf("Select(%q) = %q, want software", name, backend.Name())
		}
	}
}

func TestSelectUnknown(t *testing.T) {
	if _, err := Select("metal"); err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
}
