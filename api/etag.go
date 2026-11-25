package api

import "github.com/go-resty/resty/v2"

// ApplyETag implements the conditional-request dance shared by the resty-based
// adapters.
//
// A 304 response means the cached copy is still current, so the caller is told
// to return notModified. Otherwise the freshly returned ETag, when present and
// different from the cached one, replaces it so the next request can be
// conditional.
//
// Header lookups go through http.Header.Get, which canonicalises the name, so
// the ETag/Etag spelling difference between panels does not matter.
func ApplyETag(res *resty.Response, cache map[string]string, key string, notModified error) error {
	if res.StatusCode() == 304 {
		return notModified
	}
	if etag := res.Header().Get("ETag"); etag != "" && etag != cache[key] {
		cache[key] = etag
	}
	return nil
}
