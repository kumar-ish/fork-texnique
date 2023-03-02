# Fork-TeXnique

A (new) $\LaTeX$ speed-typesetting game. Test yourself in single-player mode, or in multi-player mode in a customizeable lobby with friends. See [credits](#credits)!

## Setup

### Running locally

To run this project, you need to have `go` installed, at the version specified in the `go.mod` file.

Afterwards, you can run the project using:

```
go run .
```

### Using Docker

If you want to use Docker to run this project, you can build an image using:

```
docker build -t forktexnique .
```

Once you've built it, you can run it using 

```
docker run -p 8080:8080 forktexnique
```

## Credits

This project was created to extend (the now relatively-unmaintained) [TeXnique](https://github.com/akshayravikumar/TeXnique) project, adding additional features and a competitions platform -- hence the name. Credits & huge props to the original creators for making it!
