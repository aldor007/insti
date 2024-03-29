FROM golang:1.16
ADD . /go/src


RUN cd /go/src/ ;go mod download; go build -o /go/insti main.go; cp -r  /go/src/static /go/; rm -rf /go/src

RUN /go/insti -h
# Run the outyet command by default when the container starts.
ENTRYPOINT ["/go/insti"]

# Expose the server TCP port
EXPOSE 8080
