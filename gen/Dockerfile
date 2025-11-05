FROM golang:1.24.5-alpine

RUN apk add --no-cache git make protoc nodejs npm libc6-compat

RUN go install goa.design/goa/v3/cmd/goa@v3.7.0 \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.30.0 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2.0 \
    && go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@v1.5.1 \
    && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.15.0 \
    && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.16.0 \
    && go install go.uber.org/mock/mockgen@v0.5.0

RUN npm install -g ts-protoc-gen protoc-gen-js

ARG GH_USER
ARG GH_TOKEN

RUN echo "machine github.com login $GH_USER password $GH_TOKEN" > /root/.netrc