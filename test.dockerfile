FROM golang:1.16 as builder

WORKDIR /go/src/csi-driver-configmap
COPY go.mod go.sum ./

RUN go mod download

COPY cmd ./cmd
COPY pkg ./pkg

RUN go test -v ./...
