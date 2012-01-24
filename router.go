package falcore

import (
	"container/list"
	"regexp"
)

// Interface for defining routers
type Router interface {
	// Returns a Pipeline or nil if one can't be found
	SelectPipeline(req *Request) (pipe RequestFilter)
}

// Interface for defining individual routes
type Route interface {
	// Returns the route's filter if there's a match.  nil if there isn't
	MatchString(str string) RequestFilter
}

// Generate a new Router instance using f for SelectPipeline
func NewRouter(f genericRouter) Router {
	return f
}

type genericRouter func(req *Request) (pipe RequestFilter)

func (f genericRouter) SelectPipeline(req *Request) (pipe RequestFilter) {
	return f(req)
}

// Will match any request.  Useful for fallthrough filters.
type MatchAnyRoute struct {
	Filter RequestFilter
}

func (r *MatchAnyRoute) MatchString(str string) RequestFilter {
	return r.Filter
}

// Will match based on a regular expression
type RegexpRoute struct {
	Match  *regexp.Regexp
	Filter RequestFilter
}

func (r *RegexpRoute) MatchString(str string) RequestFilter {
	if r.Match.MatchString(str) {
		return r.Filter
	}
	return nil
}

// Route requsts based on hostname
type HostRouter struct {
	hosts map[string]RequestFilter
}

// Generate a new HostRouter instance
func NewHostRouter() *HostRouter {
	r := new(HostRouter)
	r.hosts = make(map[string]RequestFilter)
	return r
}

// TODO: support for non-exact matches
func (r *HostRouter) AddMatch(host string, pipe RequestFilter) {
	r.hosts[host] = pipe
}

func (r *HostRouter) SelectPipeline(req *Request) (pipe RequestFilter) {
	return r.hosts[req.HttpRequest.Host]
}

// Route requests based on path
type PathRouter struct {
	Routes *list.List
}

// Generate a new instance of PathRouter
func NewPathRouter() *PathRouter {
	r := new(PathRouter)
	r.Routes = list.New()
	return r
}

func (r *PathRouter) AddRoute(route Route) {
	r.Routes.PushBack(route)
}

// convenience method for adding RegexpRoutes
func (r *PathRouter) AddMatch(match string, filter RequestFilter) (err error) {
	route := &RegexpRoute{Filter: filter}
	if route.Match, err = regexp.Compile(match); err == nil {
		r.Routes.PushBack(route)
	}
	return
}

// Will panic if r.Routes contains an object that isn't a Route
func (r *PathRouter) SelectPipeline(req *Request) (pipe RequestFilter) {
	var route Route
	for r := r.Routes.Front(); r != nil; r = r.Next() {
		route = r.Value.(Route)
		if f := route.MatchString(req.HttpRequest.URL.Path); f != nil {
			return f
		}
	}
	return nil
}
