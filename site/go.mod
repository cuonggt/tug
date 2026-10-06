module github.com/cuonggt/tug/site

go 1.26.0

require (
	github.com/alecthomas/chroma/v2 v2.20.0
	github.com/cuonggt/tug v0.0.0
	github.com/yuin/goldmark v1.8.6
)

require github.com/dlclark/regexp2 v1.11.5 // indirect

// The site says its words through tug's own package lang, as an app does,
// from this checkout.
replace github.com/cuonggt/tug => ../
