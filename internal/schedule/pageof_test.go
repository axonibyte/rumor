package schedule

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// described is a describe carrying one browser observation, as an adapter
// reports it (fable, coop r9/120).
func described(name, page, element, kind string) wire.DescribeResponse {
	return wire.DescribeResponse{Observations: []wire.Observation{
		{Name: "event_count", Source: wire.SourceAPI},
		{Name: name, Source: wire.SourceBrowser, Page: page, Element: element, Type: kind},
	}}
}

// TestAReportedPageResolvesTheSubject is the happy half of fable's r9/120: the
// adapter reports "/ui/events/{subject}" and the walk puts the observation's own
// subject in it, because the ENGINE resolves placeholders -- a driver handed a
// literal has no entity to point at, which is the same rule a flow's `navigate`
// already follows.
func TestAReportedPageResolvesTheSubject(t *testing.T) {
	page, element, kind, err := pageOf(
		described("page_count", "/ui/events/{subject}", "count", wire.ObservedNumber),
		bindings.Observation{Name: "page_count", Source: wire.SourceBrowser, Args: []string{"event"}},
		map[string]any{"event": "e-7"})
	if err != nil {
		t.Fatalf("resolving a reported page: %v", err)
	}
	if kind != wire.ObservedNumber {
		t.Errorf("the type is %q, want %q: the engine converts by this and never by appearance",
			kind, wire.ObservedNumber)
	}
	if page != "/ui/events/e-7" {
		t.Errorf("the page resolved to %q, want /ui/events/e-7", page)
	}
	if element != "count" {
		t.Errorf("the element is %q, want count", element)
	}
}

// AND A PAGE WITH NO PLACEHOLDER IS USED AS WRITTEN, which is the narrowing's
// other side: an observation whose page addresses no subject needs none, and
// requiring one would refuse a reading that is perfectly addressed.
func TestAPageWithoutASubjectIsUsedAsWritten(t *testing.T) {
	page, _, _, err := pageOf(described("page_banner", "/ui/home", "banner", wire.ObservedText),
		bindings.Observation{Name: "page_banner", Source: wire.SourceBrowser},
		map[string]any{})
	if err != nil {
		t.Fatalf("a page naming no subject was refused: %v", err)
	}
	if page != "/ui/home" {
		t.Errorf("the page is %q, want /ui/home", page)
	}
}

// TestABrowserObservationWithNoPageOrElementIsRefusedByName is the ruling's
// refusal: "a browser observation whose describe carries no page or element is
// a NAMED refusal at the walk, never an invented path or selector".
//
// BOTH HALVES SEPARATELY, because an adapter can omit either: a page with no
// element and an element with no page are different mistakes, and a refusal
// that named only one would send its reader to the wrong half.
func TestABrowserObservationWithNoPageOrElementIsRefusedByName(t *testing.T) {
	for _, c := range []struct{ name, page, element, kind string }{
		{"none of the three", "", "", ""},
		{"no element", "/ui/events/{subject}", "", wire.ObservedNumber},
		{"no page", "", "count", wire.ObservedNumber},
		{"no type", "/ui/events/{subject}", "count", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, _, err := pageOf(described("page_count", c.page, c.element, c.kind),
				bindings.Observation{Name: "page_count", Source: wire.SourceBrowser,
					Args: []string{"event"}},
				map[string]any{"event": "e-7"})
			if err == nil {
				t.Fatal("a browser observation with no address was read anyway")
			}
			for _, want := range []string{"page_count", "guessed"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal does not say %q: %v", want, err)
				}
			}
		})
	}
}

// AND A SUBJECT NOBODY NAMED IS REFUSED RATHER THAN CHOSEN. A page addresses one
// subject; an observation that resolved two arguments has not said which is the
// page's, and picking one by map order would make the address depend on Go's
// iteration.
func TestAPlaceholderWithTwoArgumentsIsRefused(t *testing.T) {
	_, _, _, err := pageOf(
		described("page_count", "/ui/events/{subject}", "count", wire.ObservedNumber),
		bindings.Observation{Name: "page_count", Source: wire.SourceBrowser,
			Args: []string{"event", "other"}},
		map[string]any{"event": "e-7", "other": "x-1"})
	if err == nil {
		t.Fatal("a page naming one subject took two arguments")
	}
	if !strings.Contains(err.Error(), "ONE subject") {
		t.Errorf("the refusal does not say a page addresses one subject: %v", err)
	}
}

// AND AN OBSERVATION THE ADAPTER NEVER DECLARED has no page at all. `rumor
// validate`'s arm 19 refuses this before any actor moves, so reaching it here is
// the build asking for what the pre-run check should have stopped -- which the
// message says rather than reading as a missing page.
func TestAnUndeclaredObservationHasNoPage(t *testing.T) {
	_, _, _, err := pageOf(
		described("page_count", "/ui/events/{subject}", "count", wire.ObservedNumber),
		bindings.Observation{Name: "page_total", Source: wire.SourceBrowser},
		map[string]any{})
	if err == nil {
		t.Fatal("an observation the adapter never declared was given a page")
	}
	if !strings.Contains(err.Error(), "declares no observation") {
		t.Errorf("the refusal does not name the absence: %v", err)
	}
}

// TestABrowserReadingConvertsByDeclarationAndNeverByAppearance is fable's
// tightening at coop r9/122 held as cells: "NO SNIFFING". The engine converts a
// `number` observation's text in one place and refuses by name any text that is
// not wholly numeric -- "0 of 2" is a refusal, not a bug and not a zero.
//
// THE TEXT CELLS ARE THE OTHER HALF, because a rule that converted everything
// would be as wrong as one that converted nothing: a `text` observation's "7"
// must STAY "7", or an attendee list showing a single name that happens to be a
// numeral would become the number seven and the statute's protection would hold
// by accident.
func TestABrowserReadingConvertsByDeclarationAndNeverByAppearance(t *testing.T) {
	for _, c := range []struct {
		name     string
		kind     string
		text     string
		children int
		want     any
	}{
		{"a figure", wire.ObservedNumber, "3", 0, 3},
		{"a figure with the template's whitespace", wire.ObservedNumber, "\n  12\n", 0, 12},
		{"a zero", wire.ObservedNumber, "0", 0, 0},
		{"a decimal reading", wire.ObservedNumber, "1.5", 0, 1.5},
		{"text stays text", wire.ObservedText, "ibis, wren", 0, "ibis, wren"},
		{"a numeral declared text stays a string", wire.ObservedText, "7", 0, "7"},
		{"text keeps its own whitespace", wire.ObservedText, " a ", 0, " a "},
		// A `count` IS THE CHILDREN AND IGNORES THE TEXT, which is the word's
		// whole point (fable, coop r9/124): a list's text is its items run
		// together, and no arithmetic on it recovers how many there were. The
		// text here is deliberately a number that is NOT the answer, so a
		// reading that fell back to parsing it would be caught.
		{"a list of three", wire.ObservedCount, "9", 3, 3},
		{"an empty list reads zero", wire.ObservedCount, "", 0, 0},
		{"a list whose text is numeric is still counted", wire.ObservedCount, "42", 1, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := observedValue("page_count", c.kind, c.text, c.children)
			if err != nil {
				t.Fatalf("%s was refused: %v", c.name, err)
			}
			if got != c.want {
				t.Errorf("read %q as %#v, want %#v", c.text, got, c.want)
			}
		})
	}
}

// AND A NUMBER THAT IS NOT WHOLLY NUMERIC IS REFUSED BY NAME. The toy's page
// renders "0 of 2" in one element, which is exactly the shape the ruling calls a
// refusal: a zero would make a page showing something else read as an empty one,
// and the raw text would reach a statute that compares counts and equal none.
func TestANumberThatIsNotWhollyNumericIsRefusedByName(t *testing.T) {
	for _, text := range []string{"0 of 2", "", "twelve", "3 attendees", "1,000", "7+"} {
		t.Run(text, func(t *testing.T) {
			got, err := observedValue("page_count", wire.ObservedNumber, text, 0)
			if err == nil {
				t.Fatalf("%q was read as %#v, and it is not wholly numeric", text, got)
			}
			if got != nil {
				t.Errorf("a refusal answered a value as well: %#v", got)
			}
			for _, says := range []string{"page_count", "wholly numeric", "ALONE"} {
				if !strings.Contains(err.Error(), says) {
					t.Errorf("the refusal does not say %q: %v", says, err)
				}
			}
		})
	}
}

// AND A TYPE THIS BUILD DOES NOT CONVERT SAYS SO ABOUT ITSELF. The wire refuses
// a third word on the describe, so reaching here is the build disagreeing with
// itself rather than an adapter's message -- which the refusal states, so its
// reader looks in the right tree.
func TestATypeThisBuildDoesNotConvertNamesItself(t *testing.T) {
	_, err := observedValue("page_count", "integer", "3", 0)
	if err == nil {
		t.Fatal("a type outside the two was converted anyway")
	}
	if !strings.Contains(err.Error(), "disagreeing with itself") {
		t.Errorf("the refusal does not name whose fault it is: %v", err)
	}
}
