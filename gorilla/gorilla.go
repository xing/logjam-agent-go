package gorilla

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/xing/logjam-agent-go"
)

// Options provides parameters to the gorilla mux.Route to logjam action name conversion.
type Options struct {
	UseRouteNames   bool // If true, logjam routes are based on route names set with the r.Name() call.
	ForceUniqueness bool // If true, setting up name extraction will panic when routes are not unique.
	CheckOnly       bool // If true, just perfroms a dry run and prints the resulting routes.
}

// Options are accessible to all functions in the package.
var opts *Options

// Routes are collected during setup and checked for uniqueness if required.
var routes = []routeInfo{}

type routeInfo struct {
	action       string     // the computed action name prefix
	route        *mux.Route // the corresponding route
	appendMethod bool       // whether to append the HTTP method to the route
}

func (ri *routeInfo) actionName() string {
	if ri.appendMethod {
		return ri.action + "#:method"
	}
	return ri.action
}

func sortRoutes() {
	sort.Slice(routes, func(i, j int) bool {
		a, _ := routes[i].route.GetPathTemplate()
		b, _ := routes[j].route.GetPathTemplate()
		return a < b
	})
}

func printRoutes() {
	fmt.Printf("\n============== logjam routes ===============\n")
	for _, r := range routes {
		methods, _ := r.route.GetMethods()
		ms := strings.Join(methods, " ")
		if ms == "" {
			ms = "ALL"
		}
		template, _ := r.route.GetPathTemplate()
		fmt.Printf("%-10s %-50s %s\n", ms, template, r.actionName())
	}
}

// SetupActionNameExtraction traverses all routes of the given router and replaces the
// corresponding handler with a new handler that uses the route name or path template to
// compute better logjam action names. It must be called after all routes have been set up
// on the router.
func SetupActionNameExtraction(r *mux.Router, options *Options) {
	opts = options
	r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		action, appendMethod := actionName(route)
		if options.CheckOnly {
			routes = append(routes, routeInfo{action: action, route: route, appendMethod: appendMethod})
		}
		if action == "" {
			return nil
		}
		route.Handler(handler{
			action:       action,
			appendMethod: appendMethod,
			handler:      route.GetHandler(),
		})
		return nil
	})
	if options.CheckOnly {
		sortRoutes()
		printRoutes()
		routes = []routeInfo{}
	}
}

type handler struct {
	action       string
	handler      http.Handler
	appendMethod bool
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logjamRequest := logjam.GetRequest(r.Context())
	if logjamRequest != nil {
		logjamRequest.Action = h.action
		if h.appendMethod {
			logjamRequest.Action += "#" + strings.ToLower(r.Method)
		}
	}
	h.handler.ServeHTTP(w, r)
}

func actionName(route *mux.Route) (string, bool) {
	template, _ := route.GetPathTemplate()
	if template == "" {
		return "", false
	}
	parts, appendMethod := actionNameParts(template)
	if appendMethod {
		return strings.Join(parts, "::"), true
	}
	action := strings.Join(parts[0:len(parts)-1], "::") + "#" + parts[len(parts)-1]
	return action, false
}

func formatSegment(s string) string {
	parts := strings.Split(s, "-")
	for i, s := range parts {
		parts[i] = strings.Title(s)
	}
	return strings.Join(parts, "")
}

func formatAction(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}

func actionNameParts(path string) ([]string, bool) {
	parts := []string{}
	splits := strings.Split(path, "/")
	n := len(splits) - 1
	lastSegmentWasPattern := false
	for i, part := range splits {
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "{") {
			if i < n {
				lastSegmentWasPattern = true
				continue
			}
			return parts, true
		}
		if i == n {
			if lastSegmentWasPattern {
				parts = append(parts, formatAction(part))
				return parts, false
			}
		}
		lastSegmentWasPattern = false
		parts = append(parts, formatSegment(part))
	}
	return parts, true
}
