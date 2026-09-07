#Schema: {
	"@schema_version"!: "v1"
	name!:              string & =~"^[a-zA-Z0-9-_]{1,64}$"
	inputs?: [...#Input]
}

#Input: {
	type!: "file" | "dir"
	name!: string & =~"^[a-zA-Z0-9-_\\.]{1,64}$"

	// Omission remains optional for v1 compatibility. Explicit false marks the
	// input as required; explicit true documents that it is optional.
	optional?: bool
}
