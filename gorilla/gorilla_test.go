package gorilla

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/golang/snappy"
	"github.com/gorilla/mux"
	"github.com/pebbe/zmq4"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/xing/logjam-agent-go"
)

func TestGorillaNameExtraction(t *testing.T) {
	router := mux.NewRouter()

	router.Path("/rest/users/{user_id}").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`some body`))
	})
	router.Path("/rest/users").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`some body`))
	}).Methods("GET", "POST")
	router.Path("/rest/schnurzes/{schnurz_id}/users").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`some body`))
	})

	fs, _ := os.Open(os.DevNull)
	logger := log.New(fs, "API", log.LstdFlags|log.Lshortfile)

	agentOptions := logjam.AgentOptions{
		Endpoints: "127.0.0.1,localhost",
		AppName:   "appName",
		EnvName:   "envName",
		Logger:    logger,
	}
	logjam.SetupAgent(&agentOptions)
	defer logjam.ShutdownAgent()

	server := httptest.NewServer(logjam.NewMiddleware(router))
	defer server.Close()

	Convey("setting up action name extraction using gorilla", t, func() {
		fmt.Println()
		SetupActionNameExtraction(router, &Options{CheckOnly: true})
	})

	Convey("extracting action names using gorilla", t, func() {
		SetupActionNameExtraction(router, &Options{})

		socket, err := zmq4.NewSocket(zmq4.ROUTER)
		So(err, ShouldBeNil)
		socket.Bind("tcp://*:9604")
		defer func() {
			socket.Unbind("tcp://*:9604")
		}()

		req, err := http.NewRequest("GET", server.URL+"/rest/users/123", nil)
		res, err := server.Client().Do(req)

		So(err, ShouldBeNil)
		So(res.StatusCode, ShouldEqual, 200)

		msg, err := socket.RecvMessage(0)
		So(err, ShouldBeNil)
		So(msg, ShouldHaveLength, 5)

		payload, err := snappy.Decode(nil, []byte(msg[3]))
		So(err, ShouldBeNil)

		output := map[string]interface{}{}
		json.Unmarshal([]byte(payload), &output)

		So(output["action"], ShouldEqual, "Rest::Users#get")

		req, err = http.NewRequest("GET", server.URL+"/rest/schnurzes/123/users", nil)
		res, err = server.Client().Do(req)

		So(err, ShouldBeNil)
		So(res.StatusCode, ShouldEqual, 200)

		msg, err = socket.RecvMessage(0)
		So(err, ShouldBeNil)
		So(msg, ShouldHaveLength, 5)

		payload, err = snappy.Decode(nil, []byte(msg[3]))
		So(err, ShouldBeNil)

		output = map[string]interface{}{}
		json.Unmarshal([]byte(payload), &output)

		So(output["action"], ShouldEqual, "Rest::Schnurzes#users")

	})
}
