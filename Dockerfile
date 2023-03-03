# syntax=docker/dockerfile:1
FROM golang:1.18-alpine AS build

WORKDIR /app

# Copy go.mod and go.sum to download dependencies
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Copy required files to run the container
COPY *.go ./

RUN export CGO_ENABLED=0 && go build -o /forktexnique

## Deploy / multistage process
FROM gcr.io/distroless/base-debian10

WORKDIR /

COPY --from=build /forktexnique /forktexnique
COPY frontend ./frontend
COPY problems.json ./

EXPOSE 8080

USER nonroot:nonroot

CMD [ "/forktexnique" ]