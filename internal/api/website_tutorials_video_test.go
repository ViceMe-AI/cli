package api

import (
	"encoding/json"
	"testing"
)

func TestWebsiteTutorialVideoLinksRoundTrip(t *testing.T) {
	const raw = `{"schemaVersion":1,"title":"教程","summary":"","prerequisites":[],"steps":[{"title":"步骤","explanation":"","prompt":"提示词","videoLinks":[{"type":"VIDEO_LINK","title":"视频","url":"https://www.bilibili.com/video/example"}]}]}`
	var content WebsiteTutorialContent
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		t.Fatal(err)
	}
	if err := content.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	var readback WebsiteTutorialContent
	if err := json.Unmarshal(data, &readback); err != nil {
		t.Fatal(err)
	}
	if len(readback.Steps[0].VideoLinks) != 1 || readback.Steps[0].VideoLinks[0].URL != "https://www.bilibili.com/video/example" {
		t.Fatal("教程视频在读取或下载时丢失")
	}
	for _, invalid := range []string{"", "invalid", "http://example.com/v", "https://user:pass@example.com/v", "https://127.0.0.1/v", "https://host.local/v"} {
		content.Steps[0].VideoLinks[0].URL = invalid
		if content.Validate() == nil {
			t.Errorf("应拒绝链接 %q", invalid)
		}
	}
}
