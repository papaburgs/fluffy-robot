package app

import (
	"net/http"
	"strings"
)

// isMobileBrowser detects mobile browsers based on User-Agent header
func isMobileBrowser(r *http.Request) bool {
	ua := r.Header.Get("User-Agent")
	mobileKeywords := []string{"Mobile", "Android", "iPhone", "iPad", "Opera Mini", "IEMobile"}
	for _, keyword := range mobileKeywords {
		if strings.Contains(ua, keyword) {
			return true
		}
	}
	return false
}
