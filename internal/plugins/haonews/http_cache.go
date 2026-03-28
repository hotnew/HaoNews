package newsplugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func marshalJSONBytes(payload any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func MarshalJSONBytes(payload any) ([]byte, error) {
	return marshalJSONBytes(payload)
}

func feedOptionsSignature(opts FeedOptions, includePage bool) string {
	values := url.Values{}
	if value := strings.TrimSpace(opts.Channel); value != "" {
		values.Set("channel", strings.ToLower(value))
	}
	if value := canonicalTopic(opts.Topic); value != "" {
		values.Set("topic", value)
	}
	if value := strings.TrimSpace(opts.Source); value != "" {
		values.Set("source", value)
	}
	if value := strings.TrimSpace(opts.Reviewer); value != "" {
		values.Set("reviewer", strings.ToLower(value))
	}
	if value := canonicalTab(opts.Tab); value != "" {
		values.Set("tab", value)
	}
	if value := strings.TrimSpace(opts.Sort); value != "" {
		values.Set("sort", strings.ToLower(value))
	}
	if value := canonicalWindow(opts.Window); value != "" {
		values.Set("window", value)
	}
	if value := strings.TrimSpace(opts.Query); value != "" {
		values.Set("q", strings.ToLower(value))
	}
	if opts.PendingApproval {
		values.Set("pending", "1")
	}
	if includePage && opts.Page > 1 {
		values.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.PageSize > 0 {
		values.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	return values.Encode()
}

func FeedOptionsSignature(opts FeedOptions, includePage bool) string {
	return feedOptionsSignature(opts, includePage)
}

func latestPostTime(posts []Post) time.Time {
	var latest time.Time
	for _, post := range posts {
		if post.CreatedAt.After(latest) {
			latest = post.CreatedAt
		}
	}
	return latest
}

func LatestPostTime(posts []Post) time.Time {
	return latestPostTime(posts)
}

func quotedETag(scope, indexSig, optionsSig string, weak bool) string {
	tag := fmt.Sprintf("%s:%s", scope, indexSig)
	if optionsSig != "" {
		tag += ":" + optionsSig
	}
	if weak {
		return `W/"` + tag + `"`
	}
	return `"` + tag + `"`
}

func QuotedETag(scope, indexSig, optionsSig string, weak bool) string {
	return quotedETag(scope, indexSig, optionsSig, weak)
}

func normalizeETag(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "W/")
	return strings.Trim(value, `"`)
}

func etagMatches(headerValue, etag string) bool {
	if strings.TrimSpace(headerValue) == "" || strings.TrimSpace(etag) == "" {
		return false
	}
	target := normalizeETag(etag)
	for _, item := range strings.Split(headerValue, ",") {
		candidate := strings.TrimSpace(item)
		if candidate == "*" {
			return true
		}
		if normalizeETag(candidate) == target {
			return true
		}
	}
	return false
}

func requestNotModified(r *http.Request, entry cachedHTTPResponse) bool {
	if r == nil {
		return false
	}
	if etagMatches(r.Header.Get("If-None-Match"), entry.etag) {
		return true
	}
	if entry.lastModified.IsZero() {
		return false
	}
	ifModifiedSince := strings.TrimSpace(r.Header.Get("If-Modified-Since"))
	if ifModifiedSince == "" {
		return false
	}
	since, err := http.ParseTime(ifModifiedSince)
	if err != nil {
		return false
	}
	return !entry.lastModified.Truncate(time.Second).After(since.Truncate(time.Second))
}

func writeConditionalResponse(w http.ResponseWriter, r *http.Request, entry cachedHTTPResponse) {
	if entry.cacheControl != "" {
		w.Header().Set("Cache-Control", entry.cacheControl)
	}
	if entry.contentType != "" {
		w.Header().Set("Content-Type", entry.contentType)
	}
	if entry.etag != "" {
		w.Header().Set("ETag", entry.etag)
	}
	if !entry.lastModified.IsZero() {
		w.Header().Set("Last-Modified", entry.lastModified.UTC().Format(http.TimeFormat))
	}
	if requestNotModified(r, entry) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	status := entry.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(entry.body)
}

func WriteConditionalResponse(w http.ResponseWriter, r *http.Request, entry cachedHTTPResponse) {
	writeConditionalResponse(w, r, entry)
}

func NewCachedHTTPResponse(status int, contentType, cacheControl, etag string, lastModified, expiresAt time.Time, body []byte) cachedHTTPResponse {
	staleUntil := expiresAt
	if seconds := staleWhileRevalidateSeconds(cacheControl); seconds > 0 && !expiresAt.IsZero() {
		staleUntil = expiresAt.Add(time.Duration(seconds) * time.Second)
	}
	return cachedHTTPResponse{
		status:       status,
		body:         append([]byte(nil), body...),
		contentType:  strings.TrimSpace(contentType),
		cacheControl: strings.TrimSpace(cacheControl),
		etag:         strings.TrimSpace(etag),
		lastModified: lastModified,
		expiresAt:    expiresAt,
		staleUntil:   staleUntil,
	}
}

func (a *App) cachedHTTPResponse(key string) (cachedHTTPResponse, bool) {
	a.responseMu.Lock()
	defer a.responseMu.Unlock()
	if a.responseCache == nil {
		return cachedHTTPResponse{}, false
	}
	entry, ok := a.responseCache[key]
	if !ok {
		return cachedHTTPResponse{}, false
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return cachedHTTPResponse{}, false
	}
	return entry, true
}

func (a *App) storeHTTPResponse(key string, entry cachedHTTPResponse) {
	a.responseMu.Lock()
	defer a.responseMu.Unlock()
	if a.responseCache == nil {
		a.responseCache = make(map[string]cachedHTTPResponse)
	}
	a.responseCache[key] = entry
}

var errCachedResponseUnavailable = errors.New("cached response unavailable")

func staleWhileRevalidateSeconds(cacheControl string) int {
	for _, part := range strings.Split(cacheControl, ",") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(strings.ToLower(part), "stale-while-revalidate=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(part, "stale-while-revalidate="))
		seconds, err := strconv.Atoi(value)
		if err == nil && seconds > 0 {
			return seconds
		}
	}
	return 0
}

func (a *App) cachedHTTPResponseStateLocked(key, variant string, now time.Time) (cachedHTTPResponse, bool, bool) {
	if a.responseCache == nil {
		return cachedHTTPResponse{}, false, false
	}
	entry, ok := a.responseCache[key]
	if !ok {
		return cachedHTTPResponse{}, false, false
	}
	variantMatch := strings.TrimSpace(variant) == "" || entry.variant == variant
	fresh := variantMatch && (entry.expiresAt.IsZero() || now.Before(entry.expiresAt))
	staleValid := fresh || entry.staleUntil.IsZero() || now.Before(entry.staleUntil)
	if !fresh && !staleValid {
		delete(a.responseCache, key)
		return cachedHTTPResponse{}, false, false
	}
	return entry, fresh, staleValid
}

func (a *App) fetchHTTPResponseVariant(key, variant string, build func() (cachedHTTPResponse, error)) (cachedHTTPResponse, error) {
	now := time.Now()
	a.responseMu.Lock()
	entry, fresh, staleValid := a.cachedHTTPResponseStateLocked(key, variant, now)
	if fresh {
		a.responseMu.Unlock()
		return entry, nil
	}
	if state, ok := a.responseBuilds[key]; ok {
		if entry, _, staleValid := a.cachedHTTPResponseStateLocked(key, variant, now); staleValid {
			a.responseMu.Unlock()
			return entry, nil
		}
		done := state.done
		a.responseMu.Unlock()
		<-done
		a.responseMu.Lock()
		err := state.err
		if entry, fresh, _ := a.cachedHTTPResponseStateLocked(key, variant, time.Now()); fresh {
			a.responseMu.Unlock()
			return entry, nil
		}
		a.responseMu.Unlock()
		if err != nil {
			return cachedHTTPResponse{}, err
		}
		return cachedHTTPResponse{}, errCachedResponseUnavailable
	}
	if a.responseBuilds == nil {
		a.responseBuilds = make(map[string]*responseBuildState)
	}
	state := &responseBuildState{done: make(chan struct{})}
	a.responseBuilds[key] = state
	epoch := a.responseEpoch
	if staleValid {
		a.responseMu.Unlock()
		go a.completeHTTPResponseBuild(key, variant, epoch, state, build)
		return entry, nil
	}
	a.responseMu.Unlock()

	entry, err := a.buildHTTPResponse(key, variant, epoch, build)
	a.responseMu.Lock()
	state.err = err
	delete(a.responseBuilds, key)
	close(state.done)
	a.responseMu.Unlock()
	return entry, err
}

func (a *App) fetchHTTPResponse(key string, build func() (cachedHTTPResponse, error)) (cachedHTTPResponse, error) {
	return a.fetchHTTPResponseVariant(key, "", build)
}

func (a *App) buildHTTPResponse(key, variant string, epoch uint64, build func() (cachedHTTPResponse, error)) (cachedHTTPResponse, error) {
	entry, err := build()
	if err != nil {
		return cachedHTTPResponse{}, err
	}
	a.responseMu.Lock()
	defer a.responseMu.Unlock()
	if a.responseEpoch == epoch {
		entry.variant = strings.TrimSpace(variant)
		if a.responseCache == nil {
			a.responseCache = make(map[string]cachedHTTPResponse)
		}
		a.responseCache[key] = entry
	}
	return entry, nil
}

func (a *App) completeHTTPResponseBuild(key, variant string, epoch uint64, state *responseBuildState, build func() (cachedHTTPResponse, error)) {
	_, err := a.buildHTTPResponse(key, variant, epoch, build)
	a.responseMu.Lock()
	defer a.responseMu.Unlock()
	state.err = err
	delete(a.responseBuilds, key)
	close(state.done)
}
