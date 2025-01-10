FROM golang:1.23 AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o simple-custom-controller .

FROM alpine:3.18
WORKDIR /
COPY --from=builder /workspace/simple-custom-controller .
COPY --from=builder /workspace/manifests ./manifests/

ENTRYPOINT ["/simple-custom-controller"]
