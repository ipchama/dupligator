FROM golang:1.15-buster as builder

ENV GOPATH=/opt/go

RUN go get -u github.com/ipchama/dupligator
RUN cd /opt/go/src/github.com/ipchama/dupligator \
    && go build -o bin/dupligator . \
    && mv bin/dupligator /usr/local/bin/dupligator

FROM debian:buster-slim
COPY --from=builder /usr/local/bin/dupligator /usr/local/bin/dupligator

ENTRYPOINT [ "dupligator"]
