FROM golang:1.15 as build

WORKDIR /app
COPY . /app

RUN go build

FROM gcr.io/distroless/base:nonroot
COPY --from=build /app/http-server /
ENTRYPOINT ["/http-server"]
