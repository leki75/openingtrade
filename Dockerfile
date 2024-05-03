FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder
RUN apk add --update --no-cache build-base git
ENV CGO_ENABLED=0
WORKDIR /go/src/github.com/alpacahq/openingtrade
COPY . .
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /go/bin ./...

FROM alpine:3.19
COPY --from=builder /go/bin/openingtrade /bin/
ENTRYPOINT ["/bin/openingtrade"]
