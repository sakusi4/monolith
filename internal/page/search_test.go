package page

import "testing"

func TestLikePattern(t *testing.T) {
	if got, want := likePattern(`50%_off\`), `%50\%\_off\\%`; got != want {
		t.Errorf("likePattern() = %q, want %q", got, want)
	}
}
