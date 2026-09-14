package s3publish

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type storedObject struct {
	Body         []byte
	CacheControl string
	ContentType  string
	ETag         string
}

type fakeS3 struct {
	mu       sync.Mutex
	objects  map[string]storedObject
	denyList bool
	puts     atomic.Int64
	heads    atomic.Int64
	gets     atomic.Int64
	lists    atomic.Int64
	deletes  atomic.Int64
	putKeys  []string
	server   *httptest.Server
}

func newFakeS3() *fakeS3 {
	fake := &fakeS3{objects: make(map[string]storedObject)}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	return fake
}

func (f *fakeS3) close() {
	f.server.Close()
}

func (f *fakeS3) endpoint() string {
	return f.server.URL
}

func (f *fakeS3) publicOrigin() string {
	return strings.TrimRight(f.server.URL, "/") + "/start"
}

func objectID(bucket, key string) string {
	return bucket + "\x00" + key
}

func (f *fakeS3) seed(bucket, key string, body []byte, cacheControl, contentType string) {
	sum := md5.Sum(body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[objectID(bucket, key)] = storedObject{
		Body:         append([]byte(nil), body...),
		CacheControl: cacheControl,
		ContentType:  contentType,
		ETag:         `"` + hex.EncodeToString(sum[:]) + `"`,
	}
}

func (f *fakeS3) get(bucket, key string) (storedObject, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	obj, ok := f.objects[objectID(bucket, key)]
	return obj, ok
}

func (f *fakeS3) putCount(bucket, key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, item := range f.putKeys {
		if item == objectID(bucket, key) {
			n++
		}
	}
	return n
}

func (f *fakeS3) serve(writer http.ResponseWriter, request *http.Request) {
	authenticated := request.Header.Get("Authorization") != "" || request.Header.Get("X-Amz-Content-Sha256") != ""
	path := strings.Trim(request.URL.Path, "/")
	query := request.URL.Query()
	bucket, key := splitBucketKey(path)

	if !authenticated {
		f.servePublic(writer, request, bucket, key, query)
		return
	}
	if query.Get("list-type") == "2" || query.Has("list-type") {
		f.serveList(writer, bucket, query.Get("prefix"))
		return
	}
	switch request.Method {
	case http.MethodHead:
		f.heads.Add(1)
		f.serveHead(writer, bucket, key)
	case http.MethodGet:
		f.gets.Add(1)
		f.serveGet(writer, bucket, key)
	case http.MethodPut:
		f.servePut(writer, request, bucket, key)
	case http.MethodDelete:
		f.deletes.Add(1)
		f.serveDelete(writer, bucket, key)
	default:
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (f *fakeS3) servePublic(writer http.ResponseWriter, request *http.Request, bucket, key string, query url.Values) {
	if query.Get("list-type") == "2" {
		http.Error(writer, "forbidden", http.StatusForbidden)
		return
	}
	if strings.HasPrefix(key, "policy-probe/") {
		http.Error(writer, "forbidden", http.StatusForbidden)
		return
	}
	switch request.Method {
	case http.MethodHead:
		f.serveHead(writer, bucket, key)
	case http.MethodGet:
		f.serveGet(writer, bucket, key)
	default:
		http.Error(writer, "forbidden", http.StatusForbidden)
	}
}

func (f *fakeS3) serveList(writer http.ResponseWriter, bucket, prefix string) {
	f.lists.Add(1)
	if f.denyList {
		writeS3Error(writer, http.StatusForbidden, "AccessDenied", "Access Denied")
		return
	}
	f.mu.Lock()
	keys := make([]string, 0)
	for id, obj := range f.objects {
		parts := strings.SplitN(id, "\x00", 2)
		if parts[0] != bucket {
			continue
		}
		if prefix != "" && !strings.HasPrefix(parts[1], prefix) {
			continue
		}
		keys = append(keys, parts[1]+"\x00"+obj.ETag+"\x00"+fmt.Sprintf("%d", len(obj.Body)))
	}
	f.mu.Unlock()
	sort.Strings(keys)
	type contents struct {
		XMLName xml.Name `xml:"Contents"`
		Key     string   `xml:"Key"`
		ETag    string   `xml:"ETag"`
		Size    int64    `xml:"Size"`
	}
	var items []contents
	for _, packed := range keys {
		parts := strings.SplitN(packed, "\x00", 3)
		size := int64(0)
		fmt.Sscanf(parts[2], "%d", &size)
		items = append(items, contents{Key: parts[0], ETag: parts[1], Size: size})
	}
	writer.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(writer, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>%s</Name><Prefix>%s</Prefix><IsTruncated>false</IsTruncated>`, xmlEscape(bucket), xmlEscape(prefix))
	for _, item := range items {
		fmt.Fprintf(writer, `<Contents><Key>%s</Key><ETag>%s</ETag><Size>%d</Size></Contents>`, xmlEscape(item.Key), xmlEscape(item.ETag), item.Size)
	}
	fmt.Fprint(writer, `</ListBucketResult>`)
}

func (f *fakeS3) serveHead(writer http.ResponseWriter, bucket, key string) {
	obj, ok := f.get(bucket, key)
	if !ok {
		writeS3Error(writer, http.StatusNotFound, "NotFound", "Not Found")
		return
	}
	f.writeObjectHeaders(writer, obj)
	writer.WriteHeader(http.StatusOK)
}

func (f *fakeS3) serveGet(writer http.ResponseWriter, bucket, key string) {
	obj, ok := f.get(bucket, key)
	if !ok {
		writeS3Error(writer, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
		return
	}
	f.writeObjectHeaders(writer, obj)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(obj.Body)
}

func (f *fakeS3) servePut(writer http.ResponseWriter, request *http.Request, bucket, key string) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		writeS3Error(writer, http.StatusBadRequest, "IncompleteBody", "Failed to read body")
		return
	}
	sum := md5.Sum(body)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	f.mu.Lock()
	f.objects[objectID(bucket, key)] = storedObject{
		Body:         body,
		CacheControl: request.Header.Get("Cache-Control"),
		ContentType:  request.Header.Get("Content-Type"),
		ETag:         etag,
	}
	f.putKeys = append(f.putKeys, objectID(bucket, key))
	f.mu.Unlock()
	f.puts.Add(1)
	writer.Header().Set("ETag", etag)
	writer.WriteHeader(http.StatusOK)
}

func (f *fakeS3) serveDelete(writer http.ResponseWriter, bucket, key string) {
	f.mu.Lock()
	delete(f.objects, objectID(bucket, key))
	f.mu.Unlock()
	writer.WriteHeader(http.StatusNoContent)
}

func (f *fakeS3) writeObjectHeaders(writer http.ResponseWriter, obj storedObject) {
	writer.Header().Set("ETag", obj.ETag)
	writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(obj.Body)))
	if obj.CacheControl != "" {
		writer.Header().Set("Cache-Control", obj.CacheControl)
	}
	if obj.ContentType != "" {
		writer.Header().Set("Content-Type", obj.ContentType)
	}
}

func splitBucketKey(path string) (string, string) {
	if path == "" {
		return "", ""
	}
	bucket, key, ok := strings.Cut(path, "/")
	if !ok {
		return path, ""
	}
	return bucket, key
}

func writeS3Error(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/xml")
	writer.Header().Set("x-amz-error-code", code)
	writer.Header().Set("x-amz-error-message", message)
	writer.WriteHeader(status)
	fmt.Fprintf(writer, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message></Error>`, xmlEscape(code), xmlEscape(message))
}

func xmlEscape(value string) string {
	var builder strings.Builder
	_ = xml.EscapeText(&builder, []byte(value))
	return builder.String()
}
