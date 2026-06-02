package getsource

import "testing"

func TestDecompressorMediaTypesConsistent(t *testing.T) {
	// Every entry in decompressorMediaTypes must have a corresponding
	// entry in Decompressors.
	for k, v := range decompressorMediaTypes {
		_, ok := Decompressors[v]
		if !ok {
			t.Errorf("decompressorMediaTypes[%q] refers to %q, which is not defined in Decompressors", k, v)
		}
	}

	// Every entry in decompressorMediaTypes must appear in
	// ociBlobMediaTypePreference.
	if len(decompressorMediaTypes) != len(ociBlobMediaTypePreference) {
		t.Errorf("decompressorMediaTypes has %d elements, ociBlobMediaTypePreference has %d; should be equal",
			len(decompressorMediaTypes), len(ociBlobMediaTypePreference))
	}
	for _, v := range ociBlobMediaTypePreference {
		_, ok := decompressorMediaTypes[v]
		if !ok {
			t.Errorf("ociBlobMediaTypePreference includes %q, not present in decompressorMediaTypes", v)
		}
	}
}

func TestGettersList(t *testing.T) {
	required := []string{"file", "git", "gcs", "s3"}
	for _, k := range required {
		if _, ok := Getters[k]; !ok {
			t.Errorf("expected Getters to contain %q", k)
		}
	}
}

func TestDecompressorsList(t *testing.T) {
	required := []string{"bz2", "gz", "xz", "zip", "tar.bz2", "tar.tbz2", "tar.gz", "tgz", "tar.xz", "txz"}
	for _, k := range required {
		if _, ok := Decompressors[k]; !ok {
			t.Errorf("expected Decompressors to contain %q", k)
		}
	}
}
