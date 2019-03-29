FROM golang
ADD . /go/src/github.com/aldor007/instagram-stats
ENV GOPATH /go

ENV DEP_VERSION v0.5.0

RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/$DEP_VERSION/dep-linux-amd64 && chmod +x /usr/local/bin/dep

RUN cd /go/src/github.com/aldor007/instagram-stats; dep ensure -vendor-only; go build -o /go/stats /go/src/github.com/aldor007/instagram-stats/main.go; cp -r  /go/src/github.com/aldor007/instagram-stats/static /go/

# Run the outyet command by default when the container starts.
ENTRYPOINT ["/go/stats"]

# Expose the server TCP port
EXPOSE 8080