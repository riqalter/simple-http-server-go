# Simple HTTP Server in Go

Simple HTTP Server is a lightweight HTTP server written in Go, designed to mimic the behavior of Python's built-in `http.server` module. This project provides a simple and efficient way to serve static files from a specified directory over HTTP.

## Features

- Serve static files from any directory
- Customizable port number
- Customizable directory to serve files from
- Easy to use command-line interface
- Cross-platform compatibility
- Verbose mode to log requests in the console
- Media viewing support (images, videos, etc.)

## Installation

you can clone this repository:

```
git clone https://github.com/riqalter/simple-http-server-go.git
cd simple-http-server-go
```

## Usage

To run the server with default settings (port 9000, current directory):

```
go run main.go
```

To specify a custom port:

```
go run main.go -port 42
```

To make it verbose (show requests in the console):

```
go run main.go -v
```

To specify a custom directory to serve files from:

```
go run main.go -dir /path/to/your/directory
```

To specify both a custom port and directory:

```
go run main.go -port 9000 -dir /path/to/your/directory
```

## Building

To build an executable:

(MacOS/Linux)
```
go build -o simplehttpserver main.go
```

(Windows)
```
go build -o simplehttpserver.exe main.go
```

This will create an executable file that you can run directly:

(MacOS/Linux)
```
./simplehttpserver -port 42 -dir /path/to/your/directory
```

(Windows)
```
./simplehttpserver.exe -port 666 -dir /path/to/your/directory
```

## Acknowledgments

This project was inspired by Python's `http.server` module and aims to provide similar functionality with Go.