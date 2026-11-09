package quotes

import "testing"

func TestRandom_ReturnsAnEntryFromTheList(t *testing.T) {
	list := []string{"a", "b", "c"}
	for i := 0; i < 50; i++ {
		got := Random(list)
		found := false
		for _, want := range list {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("Random(%v) = %q, not a member of the list", list, got)
		}
	}
}

func TestRandom_EmptyListReturnsEmptyString(t *testing.T) {
	if got := Random(nil); got != "" {
		t.Fatalf("Random(nil) = %q, want empty string", got)
	}
	if got := Random([]string{}); got != "" {
		t.Fatalf("Random([]string{}) = %q, want empty string", got)
	}
}

func TestAll_IsNotEmpty(t *testing.T) {
	if len(All) == 0 {
		t.Fatal("All must not be empty")
	}
}
