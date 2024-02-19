
## Setup

### Running locally

To run this project, you need to have `go` installed, at the version specified in the `go.mod` file.

Afterwards, you can run the project using:

```bash
go run .
```

### Using Docker

If you want to use Docker to run this project, you can build an image using:

```bash
docker build -t forktexnique .
```

Once you've built it, you can run it using 

```bash
docker run -p 8080:8080 forktexnique
```
