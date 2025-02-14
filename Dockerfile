#install packages for build layer
FROM golang:1.22.5-alpine AS builder
RUN apk add --no-cache git gcc make perl jq libc-dev linux-headers

ADD https://github.com/CosmWasm/wasmvm/releases/download/v2.1.2/libwasmvm_muslc.aarch64.a /lib/libwasmvm_muslc.aarch64.a
ADD https://github.com/CosmWasm/wasmvm/releases/download/v2.1.2/libwasmvm_muslc.x86_64.a /lib/libwasmvm_muslc.x86_64.a
RUN sha256sum /lib/libwasmvm_muslc.aarch64.a | grep 0881c5b463e89e229b06370e9e2961aec0a5c636772d5142c68d351564464a66
RUN sha256sum /lib/libwasmvm_muslc.x86_64.a | grep 58e1f6bfa89ee390cb9abc69a5bc126029a497fe09dd399f38a82d0d86fe95ef

RUN apk --print-arch > ./architecture
RUN cp /lib/libwasmvm_muslc.$(cat ./architecture).a /lib/libwasmvm_muslc.a
RUN rm ./architecture

#build binary
WORKDIR /src
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
#install binary
RUN make install

#build main container
FROM alpine:latest
RUN apk add --no-cache ca-certificates aws-cli curl tree mongodb-tools nodejs npm
RUN rm -rf /var/cache/apk/*
COPY --from=builder /go/bin/* /usr/local/bin/

#configure container
VOLUME /apps/data
WORKDIR /apps/data
