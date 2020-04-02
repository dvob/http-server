FROM golang:1.14.0 as build

WORKDIR /app
COPY . /app

RUN go build

FROM gcr.io/distroless/base
COPY --from=build /app/http-server /
ENTRYPOINT ["/http-server"]
