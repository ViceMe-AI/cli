package s3publish

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (r *regionRuntime) probeAndVerify(ctx context.Context, highest bool) error {
	probeKey := "policy-probe/" + r.cfg.RunID
	body, err := os.ReadFile(filepath.Join(r.cfg.DistDir, "agent-install.md"))
	if err != nil {
		return fmt.Errorf("%s read agent-install.md failed", r.region.Label)
	}
	if err := r.store.Put(ctx, r.region.Bucket, probeKey, body, cacheProbe, markdownType); err != nil {
		return err
	}
	probeStatus, err := r.publicStatus(ctx, strings.TrimRight(r.region.PublicOrigin, "/")+"/policy-probe/"+r.cfg.RunID)
	_ = r.store.Delete(ctx, r.region.Bucket, probeKey)
	if err != nil {
		return fmt.Errorf("%s policy probe request failed", r.region.Label)
	}
	if probeStatus == http.StatusOK {
		return fmt.Errorf("%s policy probe is publicly readable", r.region.Label)
	}
	listStatus, err := r.publicStatus(ctx, strings.TrimRight(r.region.PublicOrigin, "/")+"/?list-type=2")
	if err != nil {
		return fmt.Errorf("%s public list probe request failed", r.region.Label)
	}
	if listStatus == http.StatusOK {
		return fmt.Errorf("%s origin allows public listing", r.region.Label)
	}

	versionPrefix := strings.TrimRight(r.region.PublicOrigin, "/") + "/" + releasePrefix + "/v" + r.cfg.Version
	checks := []publicCheck{
		{URL: versionPrefix + "/agent-install.md", File: "agent-install.md", Immutable: true},
		{URL: versionPrefix + "/commerce-skill-install.md", File: "commerce-skill-install.md", Immutable: true},
		{URL: versionPrefix + "/agent-release-manifest.json", File: "agent-release-manifest.json"},
		{URL: versionPrefix + "/agent-release-manifest.sigstore.json", File: "agent-release-manifest.sigstore.json"},
	}
	if highest {
		skillsOrigin := strings.TrimSuffix(strings.TrimRight(r.region.PublicOrigin, "/"), "/start")
		checks = append(checks,
			publicCheck{URL: skillsOrigin + "/skills/manifest.json", File: filepath.Join("skills", "manifest.json")},
			publicCheck{URL: strings.TrimRight(r.region.PublicOrigin, "/") + "/agent-install.md", File: "agent-install.md", StableMarkdown: true},
			publicCheck{URL: strings.TrimRight(r.region.PublicOrigin, "/") + "/commerce-skill-install.md", File: "commerce-skill-install.md", StableMarkdown: true},
		)
	}
	for _, check := range checks {
		if err := r.verifyPublic(ctx, check); err != nil {
			return err
		}
	}
	return nil
}

type publicCheck struct {
	URL            string
	File           string
	Immutable      bool
	StableMarkdown bool
}

func (r *regionRuntime) verifyPublic(ctx context.Context, check publicCheck) error {
	local, err := os.ReadFile(filepath.Join(r.cfg.DistDir, check.File))
	if err != nil {
		return fmt.Errorf("%s read %s failed", r.region.Label, filepath.Base(check.File))
	}
	body, headers, err := r.publicGet(ctx, http.MethodGet, check.URL)
	if err != nil {
		return fmt.Errorf("%s public get %s failed: %s", r.region.Label, filepath.Base(check.File), errorCode(err))
	}
	if !bytes.Equal(body, local) {
		return fmt.Errorf("%s public object %s does not match dist", r.region.Label, filepath.Base(check.File))
	}
	if check.Immutable || check.StableMarkdown {
		_, headHeaders, headErr := r.publicGet(ctx, http.MethodHead, check.URL)
		if headErr != nil {
			return fmt.Errorf("%s public head %s failed", r.region.Label, filepath.Base(check.File))
		}
		headers = headHeaders
	}
	cache := headers.Get("Cache-Control")
	contentType := headers.Get("Content-Type")
	switch {
	case check.Immutable:
		if !strings.Contains(strings.ToLower(cache), "max-age=31536000") || !strings.Contains(strings.ToLower(cache), "immutable") {
			return fmt.Errorf("%s public object %s is missing the immutable cache header", r.region.Label, filepath.Base(check.File))
		}
	case check.StableMarkdown:
		if !strings.Contains(strings.ToLower(cache), "max-age=300") {
			return fmt.Errorf("%s public object %s is missing the stable cache header", r.region.Label, filepath.Base(check.File))
		}
		if !strings.Contains(strings.ToLower(contentType), "text/markdown") || !strings.Contains(strings.ToLower(contentType), "charset=utf-8") {
			return fmt.Errorf("%s public object %s is missing the markdown content type", r.region.Label, filepath.Base(check.File))
		}
	}
	return nil
}

func (r *regionRuntime) publicStatus(ctx context.Context, rawURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func (r *regionRuntime) publicGet(ctx context.Context, method, rawURL string) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return body, resp.Header.Clone(), nil
}

func crossVerify(ctx context.Context, cfg Config) error {
	var cn, global *Region
	for i := range cfg.Regions {
		switch cfg.Regions[i].Label {
		case "CN":
			cn = &cfg.Regions[i]
		case "GLOBAL":
			global = &cfg.Regions[i]
		}
	}
	if cn == nil || global == nil {
		return nil
	}
	cnClient, err := httpClientFor(cn.ProxyURL, defaultTimeout)
	if err != nil {
		return fmt.Errorf("CN region proxy setup failed")
	}
	globalClient, err := httpClientFor(global.ProxyURL, defaultTimeout)
	if err != nil {
		return fmt.Errorf("GLOBAL region proxy setup failed")
	}
	for _, name := range []string{"agent-install.md", "commerce-skill-install.md"} {
		cnBody, _, err := publicGet(ctx, cnClient, strings.TrimRight(cn.PublicOrigin, "/")+"/"+name)
		if err != nil {
			return fmt.Errorf("CN public get %s failed: %s", name, errorCode(err))
		}
		globalBody, _, err := publicGet(ctx, globalClient, strings.TrimRight(global.PublicOrigin, "/")+"/"+name)
		if err != nil {
			return fmt.Errorf("GLOBAL public get %s failed: %s", name, errorCode(err))
		}
		if !bytes.Equal(cnBody, globalBody) {
			return fmt.Errorf("%s differs between CN and Global", name)
		}
	}
	return nil
}

func publicGet(ctx context.Context, client *http.Client, rawURL string) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return body, resp.Header.Clone(), nil
}
