package webhook

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"strings"
	"text/template"
)

type compiledTemplates struct {
	roomKey    *template.Template
	roomName   *template.Template
	senderName *template.Template
	plain      *template.Template
	html       *template.Template
}

// FuncMap returns the template function map available in all webhook templates.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"contains": strings.Contains,
		"join": func(arr any, sep string) string {
			switch v := arr.(type) {
			case []string:
				return strings.Join(v, sep)
			case []any:
				parts := make([]string, len(v))
				for i, item := range v {
					parts[i] = fmt.Sprint(item)
				}
				return strings.Join(parts, sep)
			default:
				return fmt.Sprint(arr)
			}
		},
		"escape":    func(s any) string { return html.EscapeString(fmt.Sprint(s)) },
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"trimSpace": strings.TrimSpace,
		"default": func(fallback, value any) any {
			if value == nil || value == "" || value == 0 || value == false {
				return fallback
			}
			return value
		},
		"json": func(v any) string {
			b, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return fmt.Sprintf("<json error: %v>", err)
			}
			return string(b)
		},
		"urlEncode": func(s any) string { return url.QueryEscape(fmt.Sprint(s)) },
	}
}

// CompileTemplates compiles all template strings for a webhook configuration.
// File-based templates take precedence over inline templates.
func CompileTemplates(wh WebhookConfig) (*compiledTemplates, error) {
	ct := &compiledTemplates{}
	var err error

	ct.roomKey, err = compileOne("room_key", wh.RoomKey)
	if err != nil {
		return nil, fmt.Errorf("webhook %s room_key: %w", wh.Name, err)
	}

	roomName := wh.RoomName
	if roomName == "" {
		roomName = wh.RoomKey
	}
	ct.roomName, err = compileOne("room_name", roomName)
	if err != nil {
		return nil, fmt.Errorf("webhook %s room_name: %w", wh.Name, err)
	}

	senderName := wh.SenderName
	if senderName == "" {
		senderName = "Webhook"
	}
	ct.senderName, err = compileOne("sender_name", senderName)
	if err != nil {
		return nil, fmt.Errorf("webhook %s sender_name: %w", wh.Name, err)
	}

	plainSrc, err := loadTemplateSource(wh.Template.Plain, wh.Template.PlainFile)
	if err != nil {
		return nil, fmt.Errorf("webhook %s plain template: %w", wh.Name, err)
	}
	ct.plain, err = compileOne("plain", plainSrc)
	if err != nil {
		return nil, fmt.Errorf("webhook %s plain: %w", wh.Name, err)
	}

	htmlSrc, err := loadTemplateSource(wh.Template.HTML, wh.Template.HTMLFile)
	if err != nil {
		return nil, fmt.Errorf("webhook %s html template: %w", wh.Name, err)
	}
	if htmlSrc != "" {
		ct.html, err = compileOne("html", htmlSrc)
		if err != nil {
			return nil, fmt.Errorf("webhook %s html: %w", wh.Name, err)
		}
	}

	return ct, nil
}

func compileOne(name, src string) (*template.Template, error) {
	return template.New(name).Funcs(FuncMap()).Parse(src)
}

func loadTemplateSource(inline, file string) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read template file %s: %w", file, err)
		}
		return string(data), nil
	}
	return inline, nil
}

func renderTemplate(tmpl *template.Template, data map[string]any) (string, error) {
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}
