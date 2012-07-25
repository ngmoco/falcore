# Falcore

Falcore is a framework for constructing high performance, modular HTTP servers in Golang.

[Read more on our blog &raquo;](http://ngenuity.ngmoco.com/2012/01/introducing-falcore-and-timber.html)

[GoPkgDoc](http://gopkgdoc.appspot.com/pkg/github.com/ngmoco/falcore) hosts code documentation for this project.

## Features
* Modular and flexible design
* Hot restart hooks for zero-downtime deploys
* Builtin statistics framework
* Builtin logging framework

## Design

Falcore is a filter pipeline based HTTP server library.  You can build arbitrarily complicated HTTP services by chaining just a few simple components:
	
* `RequestFilters` are the core component.  A request filter takes a request and returns a response or nil.  Request filters an modify the request as it passes through.
* `ResponseFilters` can modify a response on its way out the door.  An example response filter, `compression_filter`, is included.  It applies `deflate` or `gzip` compression to the response if the request supplies the proper headers.
* `Pipelines` form one of the two logic components.  A pipeline contains a list of `RequestFilters` and a list of `ResponseFilters`.  A request is processed through the request filters, in order, until one returns a response.  It then passes the response through each of the response filters, in order.  A pipeline is a valid `RequestFilter`.
* `Routers` allow you to conditionally follow different pipelines.  A router chooses from a set of pipelines.  A few basic routers are included, including routing by hostname or requested path.  You can implement your own router by implementing `falcore.Router`.  `Routers` are not `RequestFilters`, but they can be put into pipelines.

## Building

Falcore is currently targeted at Go 1.0.  If you're still using Go r.60.x, you can get the last working version of falcore for r.60 using the tag `last_r60`.

Check out the project into $GOROOT/src/pkg/github.com/ngmoco/falcore.  Build using the `go build` command.

## Usage

See the `examples` directory for usage examples.

## HTTPS

To use falcore to serve HTTPS, simply call `ListenAndServeTLS` instead of `ListenAndServe`.  If you want to host SSL and nonSSL out of the same process, simply create two instances of `falcore.Server`.  You can give them the same pipeline or share pipeline components.

## Maintainers

* [Dave Grijalva](http://www.github.com/dgrijalva)
* [Scott White](http://www.github.com/smw1218)

## Contributors

* [Graham Anderson](http://www.github.com/gnanderson)
* [Amir Mohammad Saied](http://github.com/amir)
* [James Wynn](https://github.com/jameswynn)

[gb]: http://code.google.com/p/go-gb/
