package getsource

import getter "github.com/hashicorp/go-getter"

// Getters is the curated map of source getters.
// The "http", "https", and "oci" schemes are configured dynamically
// when creating a Fetcher, so they are not included here.
var Getters = map[string]getter.Getter{
	"file": new(getter.FileGetter),
	"gcs":  new(getter.GCSGetter),
	"git":  new(getter.GitGetter),
	"s3":   new(getter.S3Getter),
}
