package wire_test

import (
	"testing"

	webdriver "forge.axonibyte.com/axonibyte/rumor/internal/browser"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// TestTheWalkAndTheDriverRenderAFaultTheSameWay pins the one duplication R9.3's
// fix-up head left in the tree, and it is here because this package is the only
// one that may name BOTH types.
//
// WHY THERE ARE TWO RENDERINGS AT ALL: a fault's own words are what the
// non-stopping finding's line prints (fable, coop r9/353), so the walk needs
// them -- and internal/schedule may not name internal/browser's type, which is
// the whole reason schedule.PageFault exists beside browser.Fault. The walk
// therefore renders from its own three fields.
//
// WHY THE WORDS ARE NOT CARRIED ACROSS THE SEAM INSTEAD, measured rather than
// argued: they were, for one gate. The seam in cmd/rumor set a `Words` field
// from the driver's composer, and tests/wire went red immediately -- a fixture
// that builds schedule.PageFault field-for-field cannot know to fill a field the
// engine has started reading, which is two sources for one fact and the shape of
// the defect this project plants in the toy's `Rendered`.
//
// SO THE DUPLICATION IS DELIBERATE AND THIS ARM IS ITS PRICE. wire.UnservedKind
// is the precedent one way -- one refusal that lived in two adapters was made
// one site -- and where an import rule forbids that, the two copies get an
// oracle instead. Every kind the protocol knows is driven, including `rate`,
// which no walk produces yet: an unexercised case is exactly where a drift
// would sit unseen.
func TestTheWalkAndTheDriverRenderAFaultTheSameWay(t *testing.T) {
	for _, c := range []struct {
		what   string
		kind   string
		text   string
		status int
	}{
		{"a console entry", "console", "the page count disagrees with the store", 0},
		{"a 5xx response", "response", "/ui/events/e1/badge", 503},
		{"a request rate", "rate", "260 responses in a 2s window", 0},
		// AND THE EMPTY CASES, because a renderer that agreed only on
		// well-formed input would pass over the pair drifting on the edge a
		// real watch can hand them: a console entry with no text, and a kind
		// neither side knows.
		{"a console entry with no text", "console", "", 0},
		{"a kind neither side knows", "sideways", "whatever", 0},
	} {
		driver := webdriver.Fault{Kind: c.kind, Text: c.text, Status: c.status}.String()
		walk := schedule.PageFault{Kind: c.kind, Text: c.text, Status: c.status}.Words()
		if driver != walk {
			t.Errorf("%s: the driver says %q and the walk says %q -- two renderings of one "+
				"fault's words have drifted, and the walk's is what a reader sees on the "+
				"FOUND line", c.what, driver, walk)
		}
	}
}
