package haonews

import (
	"net/url"
	"sort"
	"strings"
)

const syncRefScheme = "haonews-sync"

func CanonicalMagnet(infoHash, displayName string) string {
	infoHash = strings.ToLower(strings.TrimSpace(infoHash))
	if infoHash == "" {
		return ""
	}
	values := url.Values{}
	values.Set("xt", "urn:btih:"+infoHash)
	displayName = strings.TrimSpace(displayName)
	if displayName != "" {
		values.Set("dn", displayName)
	}
	return "magnet:?" + values.Encode()
}

func CanonicalSyncRef(infoHash, displayName string) string {
	return canonicalSyncRefWithQuery(infoHash, syncRefQuery(displayName, nil))
}

func canonicalSyncRefWithQuery(infoHash string, values url.Values) string {
	infoHash = strings.ToLower(strings.TrimSpace(infoHash))
	if infoHash == "" {
		return ""
	}
	u := &url.URL{
		Scheme: syncRefScheme,
		Host:   "bundle",
		Path:   "/" + infoHash,
	}
	if values == nil {
		values = url.Values{}
	}
	u.RawQuery = values.Encode()
	return u.String()
}

func syncRefQuery(displayName string, extra url.Values) url.Values {
	values := url.Values{}
	displayName = strings.TrimSpace(displayName)
	if displayName != "" {
		values.Set("dn", displayName)
	}
	if extra == nil {
		return values
	}
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range extra[key] {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			values.Add(key, value)
		}
	}
	return values
}

func CanonicalizeMagnet(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	ref, err := ParseSyncRef(raw)
	if err != nil {
		return raw
	}
	displayName := magnetDisplayName(raw)
	return CanonicalMagnet(ref.InfoHash, displayName)
}

func CanonicalizeSyncRef(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	ref, err := ParseSyncRef(raw)
	if err != nil {
		return raw
	}
	displayName := syncRefDisplayName(raw)
	return CanonicalSyncRef(ref.InfoHash, displayName)
}

func magnetDisplayName(raw string) string {
	uri, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if !strings.EqualFold(uri.Scheme, "magnet") {
		return ""
	}
	return strings.TrimSpace(uri.Query().Get("dn"))
}

func syncRefDisplayName(raw string) string {
	uri, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	switch {
	case strings.EqualFold(uri.Scheme, "magnet"):
		return strings.TrimSpace(uri.Query().Get("dn"))
	case strings.EqualFold(uri.Scheme, syncRefScheme):
		return strings.TrimSpace(uri.Query().Get("dn"))
	default:
		return ""
	}
}

func canonicalMessageLink(link *MessageLink) *MessageLink {
	if link == nil {
		return nil
	}
	infoHash := strings.ToLower(strings.TrimSpace(link.InfoHash))
	magnet := CanonicalizeMagnet(link.Magnet)
	if infoHash == "" && magnet != "" {
		ref, err := ParseSyncRef(magnet)
		if err == nil {
			infoHash = ref.InfoHash
		}
	}
	if infoHash == "" && magnet == "" {
		return nil
	}
	return &MessageLink{
		InfoHash: infoHash,
		Magnet:   magnet,
	}
}
