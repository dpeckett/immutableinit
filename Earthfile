VERSION 0.8
FROM golang:1.22-bookworm
ENV DO_NOT_TRACK=1
WORKDIR /workspace

all:
  COPY (+build/matchstick --GOARCH=amd64) ./dist/matchstick-linux-amd64
  COPY (+build/matchstick --GOARCH=arm64) ./dist/matchstick-linux-arm64
  COPY (+build/matchstick --GOARCH=riscv64) ./dist/matchstick-linux-riscv64
  COPY (+package/*.deb --GOARCH=amd64) ./dist/
  COPY (+package/*.deb --GOARCH=arm64) ./dist/
  COPY (+package/*.deb --GOARCH=riscv64) ./dist/
  RUN cd dist && find . -type f | sort | xargs sha256sum >> ../sha256sums.txt
  SAVE ARTIFACT ./dist/* AS LOCAL dist/
  SAVE ARTIFACT ./sha256sums.txt AS LOCAL dist/sha256sums.txt

build:
  ARG GOOS=linux
  ARG GOARCH=amd64
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  RUN CGO_ENABLED=0 go build --ldflags "-s" -o matchstick main.go
  SAVE ARTIFACT ./matchstick AS LOCAL dist/matchstick-${GOOS}-${GOARCH}

tidy:
  LOCALLY
  ENV GOTOOLCHAIN=go1.22.1
  RUN go mod tidy
  RUN go fmt ./...

lint:
  FROM golangci/golangci-lint:v1.59.1
  WORKDIR /workspace
  COPY . ./
  RUN golangci-lint run --timeout 5m ./...

test:
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  RUN go test -coverprofile=coverage.out -v ./...
  SAVE ARTIFACT ./coverage.out AS LOCAL coverage.out

package:
  FROM debian:bookworm
  # Use bookworm-backports for newer golang versions
  RUN echo "deb http://deb.debian.org/debian bookworm-backports main" > /etc/apt/sources.list.d/backports.list
  RUN apt update
  # Tooling
  RUN apt install -y git curl devscripts dpkg-dev debhelper-compat git-buildpackage libfaketime dh-sequence-golang \
    golang-any=2:1.22~3~bpo12+1 golang-go=2:1.22~3~bpo12+1 golang-src=2:1.22~3~bpo12+1 \
    gcc-aarch64-linux-gnu gcc-riscv64-linux-gnu
  RUN curl -fsL -o /etc/apt/keyrings/apt-dpeckett-dev-keyring.asc https://apt.dpeckett.dev/signing_key.asc \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/apt-dpeckett-dev-keyring.asc] http://apt.dpeckett.dev $(. /etc/os-release && echo $VERSION_CODENAME) stable" > /etc/apt/sources.list.d/apt-dpeckett-dev.list \
    && apt update
  # Build Dependencies
  RUN apt install -y \
    golang-github-mitchellh-mapstructure-dev \
    golang-github-spf13-pflag-dev \
    golang-golang-x-sys-dev
  RUN mkdir -p /workspace/matchstick
  WORKDIR /workspace/matchstick
  COPY . .
  RUN if [ -n "$(git status --porcelain)" ]; then echo "Please commit your changes."; exit 1; fi
  RUN if [ -z "$(git describe --tags --exact-match 2>/dev/null)" ]; then echo "Current commit is not tagged."; exit 1; fi
  COPY debian/scripts/generate-changelog.sh /usr/local/bin/generate-changelog.sh
  RUN chmod +x /usr/local/bin/generate-changelog.sh
  ENV DEBEMAIL="damian@pecke.tt"
  ENV DEBFULLNAME="Damian Peckett"
  RUN /usr/local/bin/generate-changelog.sh
  RUN VERSION=$(git describe --tags --abbrev=0 | tr -d 'v') \
    && tar -czf ../matchstick_${VERSION}.orig.tar.gz --exclude-vcs .
  ARG GOARCH
  RUN dpkg-buildpackage -d -us -uc --host-arch=${GOARCH}
  SAVE ARTIFACT /workspace/*.deb AS LOCAL dist/
