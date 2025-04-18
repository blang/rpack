import "strings"

#Schema: {
    "@schema_version"!: "v1"
    source!: string & strings.MinRunes(1)
    config?: #Config
}

#Config: {
    inputs?: [string]:string
    values?: _
}

