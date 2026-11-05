package broadcast

import (
	"crypto/md5" //nolint:gosec // body_md5 is part of the Pusher HTTP API, not a security control.
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
)

// RequestSignature returns the auth_signature for a Pusher HTTP API request.
// params must not contain auth_signature. body_md5 is added when body is not empty.
func RequestSignature(secret, method, path string, params url.Values, body []byte) string {
	return HMACHex(secret, method+"\n"+path+"\n"+canonicalQuery(params, body))
}

func canonicalQuery(params url.Values, body []byte) string {
	merged := make(map[string]string, len(params)+1)
	for key, values := range params {
		if key == "auth_signature" {
			continue
		}
		merged[key] = strings.Join(values, ",")
	}
	if len(body) > 0 {
		sum := md5.Sum(body) //nolint:gosec // see import comment.
		merged["body_md5"] = hex.EncodeToString(sum[:])
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, key := range keys {
		pairs[i] = key + "=" + merged[key]
	}
	return strings.Join(pairs, "&")
}

func md5Hex(body []byte) string {
	sum := md5.Sum(body) //nolint:gosec // see import comment.
	return hex.EncodeToString(sum[:])
}
