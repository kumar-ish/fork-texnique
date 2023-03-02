FROM golang:1.18-alpine

WORKDIR /app

# Copy go.mod and go.sum to download dependencies
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Copy required files to run the container
COPY *.go ./
COPY frontend ./frontend
COPY problems.json ./

RUN go build -o /forktexnique

EXPOSE 8080
CMD [ "/forktexnique" ]

