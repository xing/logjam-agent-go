package gorilla

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/xing/logjam-agent-go"
)

// handler encapsulates the original handler and information how to determine the full
// action name for a request.
type handler struct {
	action                  string       // the precomputed action name
	appendMethod            bool         // whether the action needs to be augmented by the HTTP request method in lowercase
	handler                 http.Handler // the original handler
	methodNotAllowedHandler bool         // whether this is a method not allowed handler
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request := logjam.GetRequest(r.Context())
	if request != nil {
		request.ChangeAction(w, h.actionName(r.Method))
	}
	h.handler.ServeHTTP(w, r)
}

func (h *handler) actionName(method string) string {
	if h.appendMethod {
		if h.methodNotAllowedHandler {
			return h.action + "#methodNotAllowed"
		}
		return h.action + "#" + strings.ToLower(method)
	}
	return h.action
}

// ActionName registers a logjam action name for the given mux route. Requires a logjam
// compatible action name of the form (Module::)*Controller#action.
func ActionName(route *mux.Route, actionName string) {
	route.Handler(handler{
		action:       actionName,
		appendMethod: !strings.Contains(actionName, "#"),
		handler:      route.GetHandler(),
	})
}

var allMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

type hi struct {
	methods map[string]bool
	handler handler
}

func (i *hi) addMethods(methods []string) *hi {
	for _, m := range methods {
		i.methods[m] = true
	}
	return i
}

func (i *hi) methodComplement() []string {
	c := []string{}
	for _, m := range allMethods {
		if !i.methods[m] {
			c = append(c, m)
		}
	}
	return c
}

// AddMethodNotAllowedHandlers installs complementary handlers to the given router, for
// all logjam defined routes. It needs to be called after all routes have been added to
// the router.
func AddMethodNotAllowedHandlers(router *mux.Router) {
	handlers := map[string]*hi{}
	if router.MethodNotAllowedHandler == nil {
		router.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(405)
		})
	}
	router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		h := route.GetHandler()
		if h == nil {
			return nil
		}
		lh, isLogjamHandler := h.(handler)
		if !isLogjamHandler {
			return nil
		}
		t, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}
		methods, err := route.GetMethods()
		if err != nil {
			methods = allMethods
		}
		if oldnh, found := handlers[t]; found {
			oldnh.addMethods(methods)
			return nil
		}
		x := hi{handler: lh, methods: map[string]bool{}}
		x.addMethods(methods)
		handlers[t] = &x
		return nil
	})
	for t, hi := range handlers {
		complement := hi.methodComplement()
		if len(complement) > 0 {
			nr := router.Path(t).Methods(complement...)
			action := hi.handler.action
			if !hi.handler.appendMethod {
				action = strings.Split(action, "#")[0]
			}
			nr.Handler(handler{
				action:                  action,
				appendMethod:            true,
				handler:                 router.MethodNotAllowedHandler,
				methodNotAllowedHandler: true,
			})
		}
	}
}

// SetupRoutes traverses all routes of the given router and replaces handlers which have
// no logjam action name attached yet with a new handler that uses an action name
// derived from the path template. It must be called after all routes have been set up
// on the router.
func SetupRoutes(router *mux.Router) {
	router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		h := route.GetHandler()
		if h == nil {
			return nil
		}
		if _, isLogjamHandler := h.(handler); isLogjamHandler {
			return nil
		}
		action, appendMethod := actionName(route)
		if action == "" {
			return nil
		}
		route.Handler(handler{
			action:       action,
			appendMethod: appendMethod,
			handler:      h,
		})
		return nil
	})
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
	s = strings.Replace(s, "_", "-", -1)
	parts := strings.Split(s, "-")
	for i, s := range parts {
		parts[i] = strings.Title(s)
	}
	return strings.Join(parts, "")
}

func formatAction(s string) string {
	return strings.Replace(s, "-", "_", -1)
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

// routeInfo is just for printing routes
type routeInfo struct {
	route   *mux.Route // the corresponding route
	handler handler    // the logjam handler
}

// PrintRoutes prints the routes and their logjam action names.
func PrintRoutes(r *mux.Router) {
	routes := []routeInfo{}
	methodNotAllowedRoutes := []routeInfo{}
	r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		h := route.GetHandler()
		if h == nil {
			return nil
		}
		if lh, isLogjamHandler := h.(handler); isLogjamHandler {
			if lh.methodNotAllowedHandler {
				methodNotAllowedRoutes = append(methodNotAllowedRoutes, routeInfo{route: route, handler: lh})
			} else {
				routes = append(routes, routeInfo{route: route, handler: lh})
			}
		}
		return nil
	})
	sortRoutes(routes)
	printRoutes(routes, "====== routes ======")
	sortRoutes(methodNotAllowedRoutes)
	printRoutes(methodNotAllowedRoutes, " method not allowed ")
}

func sortRoutes(routes []routeInfo) {
	sort.Slice(routes, func(i, j int) bool {
		a, _ := routes[i].route.GetPathTemplate()
		b, _ := routes[j].route.GetPathTemplate()
		return a < b
	})
}

func printRoutes(routes []routeInfo, title string) {
	n := maxRouteLength(routes)
	fmt.Printf("\n============================%s================================\n", title)
	for _, r := range routes {
		methods, _ := r.route.GetMethods()
		if len(methods) == 0 {
			template, _ := r.route.GetPathTemplate()
			fmt.Printf("%-10s  %s  %s\n", "ALL", padRight(template, n), r.handler.actionName(":method"))
			continue
		}
		for _, m := range methods {
			template, _ := r.route.GetPathTemplate()
			fmt.Printf("%-10s  %s  %s\n", m, padRight(template, n), r.handler.actionName(m))
		}
	}
}

func maxRouteLength(routes []routeInfo) int {
	l := 0
	for _, r := range routes {
		template, _ := r.route.GetPathTemplate()
		if len(template) > l {
			l = len(template)
		}
	}
	return l
}

func padRight(s string, l int) string {
	n := len(s)
	if n >= l {
		return s
	}
	return s + strings.Repeat(" ", l-n)
}
