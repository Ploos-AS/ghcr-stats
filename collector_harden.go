package main

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var downloadsPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<h3[^>]*\btitle=["']([0-9][0-9,._ ]*)["'][^>]*>.*?</h3>.{0,1000}?<span[^>]*>\s*Total downloads\s*</span>`),
	regexp.MustCompile(`(?is)<span[^>]*>\s*Total downloads\s*</span>.{0,1000}?<h3[^>]*\btitle=["']([0-9][0-9,._ ]*)["'][^>]*>`),
}

func parseDownloadCount(text string) (int64, error) {
	for _, re := range downloadsPatterns {
		m := re.FindStringSubmatch(text)
		if len(m) != 2 {
			continue
		}
		n := strings.NewReplacer(",", "", ".", "", "_", "", " ", "").Replace(m[1])
		if v, err := strconv.ParseInt(n, 10, 64); err == nil {
			return v, nil
		}
	}
	return 0, errors.New("download count not found on GitHub package page")
}
