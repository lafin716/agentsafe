package gitlab

import (
	"net/url"
	"strings"
)

func NewMRURL(baseURL, source, target, title string) string {
	u := strings.TrimRight(baseURL, "/") + "/-/merge_requests/new"
	q := url.Values{}
	q.Set("merge_request[source_branch]", source)
	q.Set("merge_request[target_branch]", target)
	q.Set("merge_request[title]", title)
	return u + "?" + q.Encode()
}
