#install packages for build layer
FROM golang:1.21-alpine as builder
RUN apk add --no-cache git gcc make perl jq libc-dev linux-headers

ADD https://github.com/CosmWasm/wasmvm/releases/download/v1.5.0/libwasmvm_muslc.x86_64.a /lib/libwasmvm_muslc.x86_64.a
ADD https://github.com/CosmWasm/wasmvm/releases/download/v1.5.0/libwasmvm_muslc.aarch64.a /lib/libwasmvm_muslc.aarch64.a

RUN apk --print-arch > ./architecture
RUN cp /lib/libwasmvm_muslc.$(cat ./architecture).a /lib/libwasmvm_muslc.a
RUN rm ./architecture

#build binary
WORKDIR /src
COPY go.mod .
COPY go.sum .
RUN go mod tidy
COPY . .
#install binary
RUN BUILD_TAGS=muslc make install

#build main container
FROM alpine:latest
RUN apk add --update --no-cache ca-certificates
RUN apk add curl
COPY --from=builder /go/bin/* /usr/local/bin/

#configure container
VOLUME /apps/data
WORKDIR /apps/data
