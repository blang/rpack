#Schema: {
	"@schema_version"!: "v1"
	name!:              string & =~"^[a-zA-Z0-9-_]{1,64}$"
	inputs?: [...#Input]

	// Legacy pre-v0.5.0 author documentation block: never applied by any
	// released runtime (user configs provide values), tolerated so
	// definitions carrying it keep loading under the strict v1 contract.
	values?: _
}

#Input: {
	type!: "file" | "dir"
	name!: string & =~"^[a-zA-Z0-9-_\\.]{1,64}$"

	// Omission remains optional for v1 compatibility. Explicit false marks the
	// input as required; explicit true documents that it is optional.
	optional?: bool
}
