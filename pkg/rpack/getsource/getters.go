package getsource

import getter "github.com/hashicorp/go-getter"

// Getters is the curated map of source getters.
// The "http", "https", and "oci" schemes are configured dynamically
// when creating a Fetcher, so they are not included here.
//
// The git getter is wrapped so that repository-locating environment
// variables (GIT_DIR, GIT_WORK_TREE, ...) inherited from rpack's own
// environment cannot redirect the embedded git commands to a foreign
// repository. See gitGetter for details.
var Getters = map[string]getter.Getter{
	"file": new(getter.FileGetter),
	"gcs":  new(getter.GCSGetter),
	"git":  &gitGetter{inner: new(getter.GitGetter)},
	"s3":   new(getter.S3Getter),
}
