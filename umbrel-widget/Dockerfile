FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /dcrpulse-widget .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=build /dcrpulse-widget /dcrpulse-widget
USER 1000:1000
EXPOSE 3000
ENTRYPOINT ["/dcrpulse-widget"]
