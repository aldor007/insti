FROM golang
ADD . /go/insta
RUN cd /go/insta; go mod download; go build -o /go/insta/stats /go/insta/main.go

# Run the outyet command by default when the container starts.
ENTRYPOINT ["/go/insta/stats"]

# Expose the server TCP port
EXPOSE 8080