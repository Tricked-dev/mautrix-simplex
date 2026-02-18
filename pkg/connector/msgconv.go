// mautrix-simplex - A Matrix-SimpleX puppeting bridge.
// Copyright (C) 2024 Tricked
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package connector

import (
	"fmt"
	"strings"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-simplex/pkg/simplexclient"
)

// SimplexFormattedToMatrix converts a slice of SimpleX FormattedText spans into
// a plain text body and an HTML body suitable for Matrix.
func SimplexFormattedToMatrix(items []simplexclient.FormattedText) (body, html string) {
	if len(items) == 0 {
		return "", ""
	}
	var bodyBuf, htmlBuf strings.Builder
	hasFormatting := false
	for _, span := range items {
		bodyBuf.WriteString(span.Text)
		if span.Format == nil {
			htmlBuf.WriteString(escapeHTML(span.Text))
			continue
		}
		hasFormatting = true
		switch span.Format.Type {
		case "bold":
			fmt.Fprintf(&htmlBuf, "<strong>%s</strong>", escapeHTML(span.Text))
		case "italic":
			fmt.Fprintf(&htmlBuf, "<em>%s</em>", escapeHTML(span.Text))
		case "strikeThrough":
			fmt.Fprintf(&htmlBuf, "<del>%s</del>", escapeHTML(span.Text))
		case "snipped": // inline code / monospace
			fmt.Fprintf(&htmlBuf, "<code>%s</code>", escapeHTML(span.Text))
		case "uri":
			escaped := escapeHTML(span.Text)
			fmt.Fprintf(&htmlBuf, `<a href="%s">%s</a>`, escaped, escaped)
		case "email":
			escaped := escapeHTML(span.Text)
			fmt.Fprintf(&htmlBuf, `<a href="mailto:%s">%s</a>`, escaped, escaped)
		case "mention":
			// Just render as plain text; member mention resolution is complex.
			htmlBuf.WriteString(escapeHTML(span.Text))
		default:
			htmlBuf.WriteString(escapeHTML(span.Text))
		}
	}
	body = bodyBuf.String()
	if hasFormatting {
		html = htmlBuf.String()
	}
	return
}

// MatrixToSimplexMsgContent converts a Matrix message event content to a
// SimpleX MsgContent for sending. File/media types are handled separately
// in HandleMatrixMessage after downloading; this function only handles text.
func MatrixToSimplexMsgContent(content *event.MessageEventContent) simplexclient.MsgContent {
	switch content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
		text := content.Body
		if content.Format == event.FormatHTML && content.FormattedBody != "" {
			// Prefer the plain-text body since SimpleX uses its own format.
			// A full HTML→SimpleX converter is out of scope; use plain text.
			text = content.Body
		}
		return simplexclient.MsgContent{
			Type: "text",
			Text: text,
		}
	default:
		return simplexclient.MsgContent{
			Type: "text",
			Text: content.Body,
		}
	}
}

// escapeHTML escapes special HTML characters.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}
