FROM golang
ADD . /go/src/github.com/aldor007/insti
ENV GOPATH /go

ENV DEP_VERSION v0.5.0

RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/$DEP_VERSION/dep-linux-amd64 && chmod +x /usr/local/bin/dep

RUN cd /go/src/github.com/aldor007/insti; dep ensure -vendor-only; go build -o /go/insti main.go; cp -r  /go/src/github.com/aldor007/insti/static /go/

# Run the outyet command by default when the container starts.
ENTRYPOINT ["/go/insti"]

# Expose the server TCP port
EXPOSE 8080