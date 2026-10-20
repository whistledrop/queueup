package mail

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// Every email is written once, as plain text, and drawn as HTML from that
// text here. One source means the two versions can never say different
// things, and every email, including campaigns typed into the admin screen,
// gets the same finished look without anybody writing HTML.
//
// The text has a few conventions, all of which read naturally as plain text:
//
//   - A blank line starts a new paragraph. Single line breaks inside one are
//     just wrapping, and are joined up.
//   - A line "Label: https://..." with a short label is a button.
//   - A link on a line of its own is a button, labelled by the line before it
//     when that ends in a colon, or by where the link goes.
//   - Lines starting "1." or "- " are a list.
//   - The standard footer ("QueueUp is unofficial...", "Unsubscribe: ...") is
//     recognised and drawn as the footer rather than as body text.
//
// The HTML is the dull, table-based kind every mail app renders the same,
// with every style inline, because that is what survives Gmail and Outlook.

const (
	accent  = "#d05a2a"
	ink     = "#121111"
	muted   = "#5b5b66"
	line    = "#e9e9ee"
	ground  = "#f4f4f6"
	fontCSS = "-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif"
)

var (
	urlOnly     = regexp.MustCompile(`^https?://\S+$`)
	labelledURL = regexp.MustCompile(`^([^:.!?]{2,45}):\s+(https?://\S+)$`)
	inlineURL   = regexp.MustCompile(`https?://[^\s<>"]+[^\s<>".,;:!?)]`)
	orderedItem = regexp.MustCompile(`^\d+[.)]\s+`)
)

// labelFor names a button by where it goes, for a link with nothing better.
func labelFor(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "Open"
	}
	switch p := strings.TrimSuffix(u.Path, "/"); {
	case p == "/reset":
		return "Choose your password"
	case p == "/get":
		return "Get QueueUp for your PC"
	case p == "/settings":
		return "Open Settings"
	case p == "/subscribe":
		return "Go unlimited"
	case p == "/continue":
		return "Pick up where you left off"
	case p == "/schedule":
		return "Schedule a join"
	case p == "/servers":
		return "Find a server"
	case p == "/unsubscribe":
		return "Unsubscribe"
	case strings.HasPrefix(p, "/guides"):
		return "Read the guide"
	default:
		return "Open QueueUp"
	}
}

type block struct {
	kind  string // "p", "button", "ol", "ul"
	text  string
	href  string
	items []string
}

// isFooterLine reports a line of the standard footer, which is drawn
// separately.
func isFooterLine(l string) (unsub string, ok bool) {
	switch {
	case strings.HasPrefix(l, "QueueUp is unofficial"):
		return "", true
	case strings.HasPrefix(l, "Don't want these emails? Unsubscribe:"):
		return strings.TrimSpace(strings.TrimPrefix(l, "Don't want these emails? Unsubscribe:")), true
	}
	return "", false
}

// parse turns the plain text into blocks, and pulls out the unsubscribe link
// if the text carries the standard footer.
func parse(text string) (blocks []block, unsub string) {
	paras := regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(text), -1)
	for _, para := range paras {
		var lines []string
		for _, l := range strings.Split(para, "\n") {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			if u, ok := isFooterLine(l); ok {
				if u != "" {
					unsub = u
				}
				continue
			}
			lines = append(lines, l)
		}
		if len(lines) == 0 {
			continue
		}

		// A list: every line numbered, or every line a dash. A sentence
		// leading into it ("It takes two minutes:") stays a paragraph.
		lead := 0
		if len(lines) > 1 && strings.HasSuffix(lines[0], ":") && !orderedItem.MatchString(lines[0]) {
			lead = 1
		}
		rest := lines[lead:]
		if allMatch(rest, func(l string) bool { return orderedItem.MatchString(l) }) && len(rest) > 1 {
			if lead == 1 {
				blocks = append(blocks, block{kind: "p", text: lines[0]})
			}
			var items []string
			for _, l := range rest {
				items = append(items, orderedItem.ReplaceAllString(l, ""))
			}
			blocks = append(blocks, block{kind: "ol", items: items})
			continue
		}
		if allMatch(rest, func(l string) bool { return strings.HasPrefix(l, "- ") }) {
			if lead == 1 {
				blocks = append(blocks, block{kind: "p", text: lines[0]})
			}
			var items []string
			for _, l := range rest {
				items = append(items, strings.TrimPrefix(l, "- "))
			}
			blocks = append(blocks, block{kind: "ul", items: items})
			continue
		}

		// Otherwise sentences, with any button lines pulled out in place.
		var sentence []string
		flush := func() {
			if len(sentence) > 0 {
				blocks = append(blocks, block{kind: "p", text: strings.Join(sentence, " ")})
				sentence = nil
			}
		}
		for i, l := range lines {
			if m := labelledURL.FindStringSubmatch(l); m != nil {
				flush()
				blocks = append(blocks, block{kind: "button", text: m[1], href: m[2]})
				continue
			}
			if urlOnly.MatchString(l) {
				label := labelFor(l)
				// "Pick up where you left off:" on the line above names it.
				if i > 0 && len(sentence) > 0 {
					prev := sentence[len(sentence)-1]
					if strings.HasSuffix(prev, ":") && len(prev) <= 46 && len(sentence) == 1 {
						label = strings.TrimSuffix(prev, ":")
						sentence = sentence[:len(sentence)-1]
					}
				}
				flush()
				blocks = append(blocks, block{kind: "button", text: label, href: l})
				continue
			}
			sentence = append(sentence, l)
		}
		flush()
	}
	return blocks, unsub
}

func allMatch(lines []string, f func(string) bool) bool {
	if len(lines) == 0 {
		return false
	}
	for _, l := range lines {
		if !f(l) {
			return false
		}
	}
	return true
}

// inline escapes text and turns any links in it into links.
func inline(text string) string {
	var b strings.Builder
	last := 0
	for _, loc := range inlineURL.FindAllStringIndex(text, -1) {
		b.WriteString(html.EscapeString(text[last:loc[0]]))
		u := text[loc[0]:loc[1]]
		fmt.Fprintf(&b, `<a href="%s" style="color:%s;text-decoration:underline;">%s</a>`,
			html.EscapeString(u), accent, html.EscapeString(prettyURL(u)))
		last = loc[1]
	}
	b.WriteString(html.EscapeString(text[last:]))
	return b.String()
}

// prettyURL shows a link the way a person would say it: no scheme, and no
// query string full of tokens.
func prettyURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return strings.TrimSuffix(u.Host+u.Path, "/")
}

func button(label, href string) string {
	return fmt.Sprintf(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:8px 0 24px;"><tr><td align="center" bgcolor="%[1]s" style="border-radius:999px;background:%[1]s;">`+
		`<a href="%[2]s" target="_blank" style="display:inline-block;padding:14px 28px;font-family:%[3]s;font-size:16px;font-weight:600;line-height:20px;color:#ffffff;text-decoration:none;border-radius:999px;">%[4]s</a>`+
		`</td></tr></table>`, accent, html.EscapeString(href), fontCSS, html.EscapeString(label))
}

// RenderHTML draws an email from its plain text. unsubscribeURL, when set, is
// the footer's way out; when empty the text's own footer link is used, if it
// has one.
func RenderHTML(subject, text, unsubscribeURL string) string {
	blocks, found := parse(text)
	if unsubscribeURL == "" {
		unsubscribeURL = found
	}

	var body strings.Builder
	p := `margin:0 0 18px;font-family:%s;font-size:16px;line-height:26px;color:%s;`
	for _, b := range blocks {
		switch b.kind {
		case "p":
			fmt.Fprintf(&body, `<p style="`+p+`">%s</p>`, fontCSS, ink, inline(b.text))
		case "button":
			body.WriteString(button(b.text, b.href))
		case "ol", "ul":
			tag := b.kind
			fmt.Fprintf(&body, `<%s style="margin:0 0 18px;padding-left:22px;font-family:%s;font-size:16px;line-height:26px;color:%s;">`, tag, fontCSS, ink)
			for _, it := range b.items {
				fmt.Fprintf(&body, `<li style="margin:0 0 6px;">%s</li>`, inline(it))
			}
			fmt.Fprintf(&body, `</%s>`, tag)
		}
	}

	// The preview line mail apps show under the subject: the first sentence,
	// rather than whatever the HTML happens to start with.
	preheader := ""
	for _, b := range blocks {
		if b.kind == "p" {
			preheader = b.text
			break
		}
	}
	if len(preheader) > 140 {
		preheader = preheader[:140]
	}

	footer := `QueueUp is an unofficial tool and is not affiliated with Facepunch Studios.`
	links := `<a href="https://queueuprust.com" style="color:` + muted + `;text-decoration:underline;">queueuprust.com</a>`
	if unsubscribeURL != "" {
		links += ` &nbsp;·&nbsp; <a href="` + html.EscapeString(unsubscribeURL) + `" style="color:` + muted + `;text-decoration:underline;">Unsubscribe</a>`
	}

	return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light"><meta name="supported-color-schemes" content="light">
<title>` + html.EscapeString(subject) + `</title></head>
<body style="margin:0;padding:0;background:` + ground + `;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">` + html.EscapeString(preheader) + `</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:` + ground + `;">
<tr><td align="center" style="padding:32px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;">
<tr><td style="padding:0 8px 18px;font-family:` + fontCSS + `;font-size:22px;font-weight:700;letter-spacing:-0.5px;color:` + ink + `;">Queue<span style="color:` + accent + `;">Up</span></td></tr>
<tr><td style="background:#ffffff;border:1px solid ` + line + `;border-radius:20px;padding:36px 32px 18px;">
<h1 style="margin:0 0 20px;font-family:` + fontCSS + `;font-size:24px;line-height:31px;font-weight:600;letter-spacing:-0.3px;color:` + ink + `;">` + html.EscapeString(subject) + `</h1>
` + body.String() + `
</td></tr>
<tr><td style="padding:20px 8px 0;font-family:` + fontCSS + `;font-size:12px;line-height:19px;color:` + muted + `;">` + footer + `<br>` + links + `</td></tr>
</table></td></tr></table></body></html>`
}
