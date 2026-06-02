package getsource

import getter "github.com/hashicorp/go-getter"

// Decompressors is the curated map of archive decompressors.
var Decompressors = map[string]getter.Decompressor{
	"bz2": new(getter.Bzip2Decompressor),
	"gz":  new(getter.GzipDecompressor),
	"xz":  new(getter.XzDecompressor),
	"zip": new(getter.ZipDecompressor),

	"tar.bz2":  new(getter.TarBzip2Decompressor),
	"tar.tbz2": new(getter.TarBzip2Decompressor),
	"tar.gz":   new(getter.TarGzipDecompressor),
	"tgz":      new(getter.TarGzipDecompressor),
	"tar.xz":   new(getter.TarXzDecompressor),
	"txz":      new(getter.TarXzDecompressor),
}

// decompressorMediaTypes maps OCI media types to decompressor keys in
// the Decompressors map.
var decompressorMediaTypes = map[string]string{
	"archive/zip": "zip",
}

// ociBlobMediaTypePreference defines our preference order for the media
// types of OCI blobs representing module packages. When multiple layers
// are found in a manifest, the first matching type in this list is used.
var ociBlobMediaTypePreference = []string{
	"archive/zip",
}
