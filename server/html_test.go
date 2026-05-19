package server

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
	"time"

	"grunt/client"
)

func TestLoginTemplate_Parses(t *testing.T) {
	if loginTmpl == nil {
		t.Fatal("loginTmpl should not be nil")
	}
}

func TestLoginTemplate_NoError(t *testing.T) {
	var buf bytes.Buffer
	err := loginTmpl.Execute(&buf, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "Grunt Login") {
		t.Error("missing title")
	}
	if strings.Contains(html, "error-message") {
		t.Error("should not contain error message when no error provided")
	}
}

func TestLoginTemplate_WithError(t *testing.T) {
	var buf bytes.Buffer
	err := loginTmpl.Execute(&buf, map[string]string{"Error": "Invalid credentials"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "Invalid credentials") {
		t.Error("missing error message in output")
	}
	if !strings.Contains(html, "error-message") {
		t.Error("missing error-message class")
	}
}

func TestChatTemplate_Parses(t *testing.T) {
	if chatTmpl == nil {
		t.Fatal("chatTmpl should not be nil")
	}
}

func TestChatTemplate_RenderWithMessages(t *testing.T) {
	msgs := []MessageTemplateData{
		{
			ID:            1,
			User:          "alice",
			Content:       "hello",
			RenderedContent: "hello",
			Timestamp:     "14:30",
			Color:         "#3b82f6",
			ShowAvatar:    true,
			ShowUsername:  true,
			ShowTimestamp: true,
		},
	}
	data := ChatData{
		Messages: msgs,
		Profile: AvatarData{
			UserID:    "alice",
			URL:       "/settings",
			Color:     "#3b82f6",
			TextColor: "#fff",
			Initial:   "A",
		},
	}

	var buf bytes.Buffer
	err := chatTmpl.Execute(&buf, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "hello") {
		t.Error("missing message content")
	}
	if !strings.Contains(html, "alice") {
		t.Error("missing username")
	}
	if !strings.Contains(html, `data-id="1"`) {
		t.Error("missing message ID")
	}
	if !strings.Contains(html, "/settings") {
		t.Error("missing profile link")
	}
}

func TestChatTemplate_RenderEmptyMessages(t *testing.T) {
	data := ChatData{
		Messages: []MessageTemplateData{},
		Profile: AvatarData{
			UserID:    "bob",
			URL:       "/settings",
			Color:     "#3b82f6",
			TextColor: "#fff",
			Initial:   "B",
		},
	}

	var buf bytes.Buffer
	err := chatTmpl.Execute(&buf, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "Grunt Chat") {
		t.Error("missing title")
	}
}

func TestSettingsTemplate_Parses(t *testing.T) {
	if settingsTmpl == nil {
		t.Fatal("settingsTmpl should not be nil")
	}
}

func TestSettingsTemplate_Render(t *testing.T) {
	data := map[string]AvatarData{
		"Profile": {
			UserID:    "charlie",
			URL:       "/",
			Color:     "#3b82f6",
			TextColor: "#fff",
			Initial:   "C",
		},
	}

	var buf bytes.Buffer
	err := settingsTmpl.Execute(&buf, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "Grunt Settings") {
		t.Error("missing title")
	}
	if !strings.Contains(html, "/") {
		t.Error("missing profile link")
	}
}

func TestAvatarData_Struct(t *testing.T) {
	a := AvatarData{
		UserID:    "test",
		URL:       "/link",
		Color:     "#ff0000",
		TextColor: "#ffffff",
		Initial:   "T",
	}
	if a.UserID != "test" {
		t.Error("UserID mismatch")
	}
	if a.URL != "/link" {
		t.Error("URL mismatch")
	}
}

func TestMessageTemplateData_Struct(t *testing.T) {
	m := MessageTemplateData{
		ID:        42,
		User:      "user",
		Content:   "content",
		Timestamp: "12:00",
		Color:     "#00ff00",
	}
	if m.ID != 42 {
		t.Error("ID mismatch")
	}
	if m.User != "user" {
		t.Error("User mismatch")
	}
	if m.Content != "content" {
		t.Error("Content mismatch")
	}
}

func TestAvatarColor_Deterministic(t *testing.T) {
	c1 := avatarColor("alice")
	c2 := avatarColor("alice")
	if c1 != c2 {
		t.Error("avatar color should be deterministic for same user")
	}
	c3 := avatarColor("bob")
	if c1 == c3 {
		t.Error("different users should have different colors")
	}
}

func TestAvatarTextColor_Deterministic(t *testing.T) {
	c1 := avatarTextColor("alice")
	c2 := avatarTextColor("alice")
	if c1 != c2 {
		t.Error("text color should be deterministic for same user")
	}
}

func TestChatData_Struct(t *testing.T) {
	msgs := []MessageTemplateData{
		{ID: 1, User: "u", Content: "c", Timestamp: "t", Color: "#000"},
	}
	data := ChatData{
		Messages: msgs,
		Profile: AvatarData{UserID: "p", URL: "/u", Color: "#fff", TextColor: "#000", Initial: "P"},
	}
	if len(data.Messages) != 1 {
		t.Error("messages length mismatch")
	}
	if data.Profile.UserID != "p" {
		t.Error("profile mismatch")
	}
}

func TestRenderMessageHTMLTemplate(t *testing.T) {
	bcast := client.Broadcast{
		ID:        99,
		UserID:    "eve",
		Content:   "test message",
		Timestamp: time.Now(),
	}

	result := renderMessageHTMLTemplate(bcast, true, true, true)
	if !strings.Contains(result, "test message") {
		t.Error("missing content in rendered message")
	}
	if !strings.Contains(result, `data-id="99"`) {
		t.Error("missing ID in rendered message")
	}
	if !strings.Contains(result, "eve") {
		t.Error("missing username in rendered message")
	}
}

func TestRenderMessageHTMLTemplate_EscapesContent(t *testing.T) {
	bcast := client.Broadcast{
		ID:        100,
		UserID:    "<script>alert('xss')</script>",
		Content:   "<img onerror=alert(1)>",
		Timestamp: time.Now(),
	}

	result := renderMessageHTMLTemplate(bcast, true, true, true)
	if strings.Contains(result, "<script>") {
		t.Error("raw <script> should be escaped in output")
	}
	if strings.Contains(result, "<img ") {
		t.Error("raw <img > tag should be escaped in output")
	}
	if !strings.Contains(result, "&lt;script&gt;") {
		t.Error("output should contain escaped &lt;script&gt;")
	}
	if !strings.Contains(result, "&lt;img onerror=alert(1)&gt;") {
		t.Error("output should contain properly escaped img tag with onerror attribute")
	}
}

func TestRenderMessageHTMLTemplate_MultilineContent(t *testing.T) {
	bcast := client.Broadcast{
		ID:        200,
		UserID:    "multiline",
		Content:   "line one\nline two\nline three",
		Timestamp: time.Now(),
	}

result := renderMessageHTMLTemplate(bcast, true, true, true)
	if !strings.Contains(result, "line one") {
		t.Error("missing first line in rendered message")
	}
	if !strings.Contains(result, "line two") {
		t.Error("missing second line in rendered message")
	}
	if !strings.Contains(result, "line three") {
		t.Error("missing third line in rendered message")
	}
	if !strings.Contains(result, "<br>") {
		t.Error("newlines should be replaced with <br> tags for SSE compatibility")
	}
	if strings.Contains(result, "\n") {
		t.Error("rendered HTML must not contain literal newline characters")
	}
}

func TestAllTemplates_Parse(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    *template.Template
		wantNil bool
	}{
		{"login", loginTmpl, false},
		{"chat", chatTmpl, false},
		{"settings", settingsTmpl, false},
		{"message", messageTmpl, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if (tt.tmpl == nil) != tt.wantNil {
				t.Errorf("%s template is nil", tt.name)
			}
		})
	}
}
