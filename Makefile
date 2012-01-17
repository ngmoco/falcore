include $(GOROOT)/src/Make.inc

TARG=falcore
GOFILES= \
        filter.go \
				logger.go \
				pipeline.go \
				request.go \
				response.go \
				router.go \
				server.go \
				string_body.go

include $(GOROOT)/src/Make.pkg
