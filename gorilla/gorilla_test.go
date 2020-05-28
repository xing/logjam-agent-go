package gorilla

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/snappy"
	"github.com/gorilla/mux"
	"github.com/pebbe/zmq4"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/xing/logjam-agent-go"
)

func TestGorillaNameExtraction(t *testing.T) {
	router := mux.NewRouter()

	somebody := func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`some body`))
	}

	ActionName(router.Path("/rest/users").Methods("GET").HandlerFunc(somebody), "Rest::Users#index")
	ActionName(router.Path("/rest/users").Methods("POST").HandlerFunc(somebody), "Rest::Users#create")
	ActionName(router.Path("/rest/users/{user_id}").Methods("GET").HandlerFunc(somebody), "Rest::Users#show")
	ActionName(router.Path("/rest/users/{user_id}").Methods("PUT", "PATCH").HandlerFunc(somebody), "Rest::Users#update")
	ActionName(router.Path("/rest/users/{user_id}").Methods("DELETE").HandlerFunc(somebody), "Rest::Users#destroy")
	ActionName(router.Path("/rest/users/{user_id}/comrades").Methods("GET").HandlerFunc(somebody), "Rest::Users#comrades")

	sub := router.PathPrefix("/web").Subrouter()
	ActionName(sub.Path("/users").Methods("GET").HandlerFunc(somebody), "Web::Users#index")
	ActionName(sub.Path("/users").Methods("POST").HandlerFunc(somebody), "Web::Users#create")
	ActionName(sub.Path("/users/{user_id}").Methods("GET").HandlerFunc(somebody), "Web::Users#show")
	ActionName(sub.Path("/users/{user_id}").Methods("PUT", "PATCH").HandlerFunc(somebody), "Web::Users#update")
	ActionName(sub.Path("/users/{user_id}").Methods("DELETE").HandlerFunc(somebody), "Web::Users#destroy")
	ActionName(sub.Path("/users/{user_id}/comrades").Methods("GET").HandlerFunc(somebody), "Web::Users#comrades")

	router.Path("/allmethods").HandlerFunc(somebody)
	router.Path("/simple").Methods("GET").HandlerFunc(somebody)

	agentOptions := logjam.Options{
		Endpoints: "127.0.0.1,localhost",
		AppName:   "appName",
		EnvName:   "envName",
		Logger:    log.New(ioutil.Discard, "", 0),
	}
	agent := logjam.NewAgent(&agentOptions)
	defer agent.Shutdown()

	server := httptest.NewServer(agent.NewMiddleware(router))
	defer server.Close()

	socket, err := zmq4.NewSocket(zmq4.ROUTER)
	if err != nil {
		panic("cannot bind socket for testing")
	}
	socket.Bind("tcp://*:9604")
	defer func() {
		socket.Unbind("tcp://*:9604")
	}()

	performAndCheck := func(method string, path string, expectedResonseCode int, expectedActionName string) {
		req, err := http.NewRequest(method, server.URL+path, nil)
		res, err := server.Client().Do(req)

		So(err, ShouldBeNil)
		So(res.StatusCode, ShouldEqual, expectedResonseCode)

		msg, err := socket.RecvMessage(0)
		So(err, ShouldBeNil)
		So(msg, ShouldHaveLength, 5)

		payload, err := snappy.Decode(nil, []byte(msg[3]))
		So(err, ShouldBeNil)

		output := map[string]interface{}{}
		json.Unmarshal([]byte(payload), &output)

		So(output["action"], ShouldEqual, expectedActionName)
	}

	Convey("setting up action name extraction using gorilla", t, func() {

		SetupRoutes(router)
		PrintRoutes(router)

	})

	Convey("defining action names using gorilla", t, func() {

		SetupRoutes(router)

		performAndCheck("GET", "/rest/users/123", 200, "Rest::Users#show")
		performAndCheck("DELETE", "/rest/users/123", 200, "Rest::Users#destroy")
		performAndCheck("PUT", "/rest/users/123", 200, "Rest::Users#update")
		performAndCheck("GET", "/rest/users/123/comrades", 200, "Rest::Users#comrades")
		// performAndCheck("GET", "/web", 404, "Unknown#web")
		performAndCheck("GET", "/simple", 200, "Simple#get")
		performAndCheck("POST", "/allmethods", 200, "Allmethods#post")

	})
}
