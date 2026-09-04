package main

import "regexp"

var hardenedDownloadsPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<h3[^>]*\btitle=["']([0-9][0-9,._ ]*)["'][^>]*>.*?</h3>.{0,2000}?<span[^>]*>\s*Total downloads\s*</span>`),
	regexp.MustCompile(`(?is)<span[^>]*>\s*Total downloads\s*</span>.{0,2000}?<h3[^>]*\btitle=["']([0-9][0-9,._ ]*)["'][^>]*>`),
}

func init() {
	// GitHub package pages may contain unrelated text that looks like
	// "N downloads". Restrict extraction to the package statistics card
	// explicitly labelled "Total downloads" and read the exact value from
	// the adjacent h3 title attribute.
	downloadsPatterns = hardenedDownloadsPatterns
}
