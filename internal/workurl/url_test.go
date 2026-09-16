package workurl

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"
)

func TestPublicWorkURLContract(t *testing.T) {
	data, err := os.ReadFile("testdata/public-work-urls.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		URL, Handle, Slug string
		Valid             bool
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.URL, func(t *testing.T) {
			parsed, err := url.Parse(tc.URL)
			if err != nil {
				t.Fatal(err)
			}
			before := parsed.String()
			handle, slug, valid := PublicParts(parsed)
			if valid != tc.Valid || (valid && (handle != tc.Handle || slug != tc.Slug)) {
				t.Fatalf("got %q %q %t", handle, slug, valid)
			}
			if parsed.String() != before {
				t.Fatal("parser mutated product/install/origin state")
			}
		})
	}
}

func TestPublicWorkURLRelativeAndAuthorityBoundary(t *testing.T) {
	for raw, valid := range map[string]bool{
		"/viceme?workSlug=use-a-skill":                        true,
		"//evil.example/alice?workSlug=site":                  false,
		"https://user:password@viceme.cn/alice?workSlug=site": false,
		"https:///alice?workSlug=site":                        false,
		"ftp://viceme.cn/alice?workSlug=site":                 false,
		"alice?workSlug=site":                                 false,
	} {
		parsed, _ := url.Parse(raw)
		if _, _, got := PublicParts(parsed); got != valid {
			t.Errorf("%s: %t", raw, got)
		}
	}
}

func TestDisplayOnlyOmitsPublicDefaults(t *testing.T) {
	for _, base := range []string{"https://viceme.cn", "https://viceme.ai", "https://dev.viceme.cn", ""} {
		for _, ext := range []string{"", ".md"} {
			full := base + "/alice" + ext + "?mode=consumer&view=work&workSlug=site&product=p%31&install=owned#readme"
			want := base + "/alice" + ext + "?workSlug=site&product=p%31&install=owned#readme"
			if got := Display(full); got != want {
				t.Errorf("got %s, want %s", got, want)
			}
			if Display(want) != want {
				t.Fatal("short link is not idempotent")
			}
		}
	}
	for _, raw := range []string{
		"/alice?mode=creator&view=work&workSlug=site", "/alice?mode=consumer&view=discover&workSlug=site",
		"/alice?mode=consumer&view=work&workSlug=site&signature=opaque", "/alice?mode=consumer&view=work&workSlug=site&workSlug=other",
		"/alice?mode=consumer&view=work&workSlug=site&action=preview", "/alice", "/alice/site", "/README.md", "https://s3.viceme.cn/download?signature=opaque",
	} {
		if got := Display(raw); got != raw {
			t.Errorf("rewrote %s to %s", raw, got)
		}
	}
}
