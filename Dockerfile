FROM golang:1.16 as builder

WORKDIR /go/src/csi-driver-configmap
COPY go.mod go.sum ./

RUN go mod download

COPY cmd ./cmd
COPY pkg ./pkg

RUN CGO_ENABLED=0 go build -o csi-configmap-plugin ./cmd/plugin

FROM alpine:3
WORKDIR /
COPY --from=builder /go/src/csi-driver-configmap/csi-configmap-plugin ./
ENTRYPOINT ["/csi-configmap-plugin"]
