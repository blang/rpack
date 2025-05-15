# Partially adapted from https://github.com/crazywolf132/ultimate-gojust

# Use bash with strict error checking
set shell := ["bash", "-uc"]

# Version control
# Automatically detect version information from git
# Falls back to timestamp if not in a git repository
version := `git describe --tags --always 2>/dev/null || echo "dev"`
git_commit := `git rev-parse --short HEAD 2>/dev/null || echo "unknown"`
build_time := `date -u '+%Y-%m-%d_%H:%M:%S'`

# Build flags
# Linker flags for embedding version information
ld_flags := "-s -w \
    -extldflags=-static \
    -X '$(go list -m)/pkg/cmd.BuildVersion=" + version + "' \
    -X '$(go list -m)/pkg/cmd.BuildCommit=" + git_commit + "' \
    -X '$(go list -m)/pkg/cmd.BuildTime=" + build_time + "'"

# Directories
# Project directory structure
root_dir := justfile_directory()
bin_dir := root_dir + "/bin"
dist_dir := root_dir + "/dist"
dist_name := "dist"

default: build

dev: goimports build run

fix:
    just goimports
    just tidy

goimports:
    goimports -w ./

check:
    go vet ./...
    golangci-lint run

ldoc:
    cd ./lua && ldoc ./src -d ./docs/gen

vet:

test:
    go test -v ./...

testlua:
    go test -v -test.run '.*Lua.*' ./...

tidy:
    go mod tidy

run:
    ./rpack

example: build
    ./rpack run ./examples/basic/root/basic.rpack.yaml
    

build platform="linux/amd64/-":
    #!/usr/bin/env sh
    mkdir -p "{{dist_dir}}"
    platform="{{platform}}"
    echo "Platform: $platform"
    os=$(echo $platform | cut -d/ -f1)
    arch=$(echo $platform | cut -d/ -f2)
    arm=$(echo $platform | cut -d/ -f3)
    output="{{dist_name}}/rpack-${os}-${arch}"

    CGO_ENABLED="0" GOOS=$os GOARCH=$arch $([ "$arm" != "-" ] && echo "GOARM=$arm") \
    go build \
        -ldflags '{{ld_flags}}' \
        -o "$output" \
        ./cmd/rpack

build-all:
    #!/usr/bin/env sh
    mkdir -p "{{dist_dir}}"
    for platform in \
        "linux/amd64/-" \
        "linux/arm64/-" \
        "darwin/amd64/-" \
        "darwin/arm64/-"; do
        os=$(echo $platform | cut -d/ -f1)
        arch=$(echo $platform | cut -d/ -f2)
        arm=$(echo $platform | cut -d/ -f3)
        binary="rpack-${os}-${arch}" 
        output="{{dist_dir}}/rpack-${os}-${arch}"
        
        CGO_ENABLED=0 GOOS=$os GOARCH=$arch $([ "$arm" != "-" ] && echo "GOARM=$arm") \
        go build \
            -ldflags '{{ld_flags}}' \
            -o "$output" \
            ./cmd/rpack
        
        tar -C "{{dist_dir}}" -czf "$output.tar.gz" "$binary"
    done

