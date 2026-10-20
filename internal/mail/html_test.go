package mail

import (
	"strings"
	"testing"
)

func TestALabelledLinkBecomesAButton(t *testing.T) {
	out := RenderHTML("Hi", "Ten joins a month.\n\nGo unlimited: https://queueuprust.com/subscribe\n", "")
	if !strings.Contains(out, `href="https://queueuprust.com/subscribe"`) || !strings.Contains(out, ">Go unlimited</a>") {
		t.Fatalf("no Go unlimited button in:\n%s", out)
	}
	if strings.Contains(out, "Go unlimited: https") {
		t.Fatal("the button's line was also printed as text")
	}
}

// "Pick up where you left off:" on the line above a bare link names the button.
func TestTheLineAboveALinkNamesIt(t *testing.T) {
	out := RenderHTML("Hi", "Pick up where you left off:\nhttps://queueuprust.com/continue?a=1&t=2\n", "")
	if !strings.Contains(out, ">Pick up where you left off</a>") {
		t.Fatalf("button not named by the line above:\n%s", out)
	}
	// The ampersand in the address is escaped in the attribute.
	if !strings.Contains(out, `continue?a=1&amp;t=2`) {
		t.Fatal("the link's query string was not escaped")
	}
}

func TestABareLinkIsNamedByWhereItGoes(t *testing.T) {
	out := RenderHTML("Reset", "Open this link to choose your password:\n\nhttps://queueuprust.com/reset?token=abc\n", "")
	if !strings.Contains(out, ">Choose your password</a>") {
		t.Fatalf("reset link not named:\n%s", out)
	}
}

// A link in the middle of a sentence stays a link, shown without its scheme
// or query string.
func TestALinkInASentenceStaysInline(t *testing.T) {
	out := RenderHTML("Hi", "Read more at https://queueuprust.com/help?x=1 when you like.", "")
	if !strings.Contains(out, ">queueuprust.com/help</a> when you like.") {
		t.Fatalf("inline link wrong:\n%s", out)
	}
	if strings.Contains(out, "border-radius:999px;background") && strings.Count(out, "padding:14px 28px") > 0 {
		t.Fatal("an inline link became a button")
	}
}

func TestWrappedLinesAreJoinedAndListsAreLists(t *testing.T) {
	text := "It takes about two minutes:\n  1. Download it.\n  2. Read the code.\n  3. Type it in.\n\nThen you are\nready to go."
	out := RenderHTML("Hi", text, "")
	if !strings.Contains(out, "<ol") || strings.Count(out, "<li") != 3 {
		t.Fatalf("numbered lines did not become a list:\n%s", out)
	}
	if !strings.Contains(out, "Then you are ready to go.") {
		t.Fatal("wrapped lines were not joined")
	}
}

// Anything typed into a campaign is text, never markup.
func TestTextCannotInjectMarkup(t *testing.T) {
	out := RenderHTML(`<b>Sub</b>`, `<script>alert(1)</script> & "quotes"`, "")
	if strings.Contains(out, "<script>") || strings.Contains(out, "<b>Sub</b>") {
		t.Fatalf("markup got through:\n%s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatal("text was not escaped")
	}
}

// The standard footer is drawn as the footer, with a real unsubscribe link,
// and not repeated in the body.
func TestTheFooterIsRecognised(t *testing.T) {
	text := "Hello.\n\nQueueUp is unofficial and not affiliated with Facepunch Studios.\n" +
		"Don't want these emails? Unsubscribe: https://queueuprust.com/unsubscribe?a=1&t=2\n"
	out := RenderHTML("Hi", text, "")
	if strings.Contains(out, "Don&#39;t want these emails") {
		t.Fatal("footer text left in the body")
	}
	if !strings.Contains(out, `href="https://queueuprust.com/unsubscribe?a=1&amp;t=2"`) || !strings.Contains(out, ">Unsubscribe</a>") {
		t.Fatalf("no unsubscribe link in the footer:\n%s", out)
	}
}
