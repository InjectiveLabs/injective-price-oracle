#install packages for build layer
FROM golang:1.24.0-alpine AS builder
RUN apk add --no-cache git gcc make perl jq libc-dev linux-headers

ADD https://github.com/CosmWasm/wasmvm/releases/download/v2.1.5/libwasmvm_muslc.aarch64.a /lib/libwasmvm_muslc.aarch64.a
ADD https://github.com/CosmWasm/wasmvm/releases/download/v2.1.5/libwasmvm_muslc.x86_64.a /lib/libwasmvm_muslc.x86_64.a
RUN sha256sum /lib/libwasmvm_muslc.aarch64.a | grep 1bad0e3f9b72603082b8e48307c7a319df64ca9e26976ffc7a3c317a08fe4b1a
RUN sha256sum /lib/libwasmvm_muslc.x86_64.a | grep c6612d17d82b0997696f1076f6d894e339241482570b9142f29b0d8f21b280bf

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
RUN make install

#build main container
FROM alpine:3.22.2
RUN apk add --no-cache ca-certificates aws-cli curl tree mongodb-tools nodejs npm
RUN rm -rf /var/cache/apk/*
COPY --from=builder /go/bin/* /usr/local/bin/
COPY --from=builder /src/swagger /apps/data/swagger

#configure container
VOLUME /apps/data
WORKDIR /apps/data
