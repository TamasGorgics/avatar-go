# syntax=docker/dockerfile:1

FROM golang:1.27.1-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/avatar .

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/avatar /avatar

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/avatar"]
