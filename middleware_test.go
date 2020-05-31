package logjam

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"log"

	"github.com/golang/snappy"
	"github.com/gorilla/mux"
	"github.com/pebbe/zmq4"
	. "github.com/smartystreets/goconvey/convey"
)

func TestObfuscateIP(t *testing.T) {
	Convey("Obfuscate IP", t, func() {
		ips := map[string]string{
			"0000:0000:0000:0000:0000:FFFF:C0A8:1": "192.168.0.XXX",
			"192.168.0.1":                          "192.168.0.XXX",
			"::FFFF:192.168.0.1":                   "192.168.0.XXX",
			"::FFFF:C0A8:1":                        "192.168.0.XXX",
			"fe80::da50:e6ff:fedb:c252":            "fe80::da50:e6ff:fedb:XXXX",
			"::fedb:c252":                          "::fedb:XXXX",
			"invalid":                              "invalid",
		}

		// this is mostly so test order is always deterministic
		keys := []string{}
		for k := range ips {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			Convey(key, func() {
				So(obfuscateIP(key), ShouldEqual, ips[key])
			})
		}
	})
}

type recoveryHandler struct {
	handler http.Handler
}

func (h recoveryHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer func() { recover() }()
	h.handler.ServeHTTP(w, req)
}

func TestMiddleware(t *testing.T) {
	os.Setenv("HOSTNAME", "test-machine")
	os.Setenv("DATACENTER", "dc")
	os.Setenv("CLUSTER", "a")
	os.Setenv("NAMESPACE", "logjam")
	setRequestEnv()

	router := mux.NewRouter()
	logger := Logger{Logger: log.New(ioutil.Discard, "", 0)}

	router.Path("/rest/app/vendor/v1/users/123").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		logger.Debug(ctx, "First Line")
		logger.Info(ctx, "Second Line")
		logger.Warn(ctx, "Third Line")
		logger.Error(ctx, "Fourth Line")

		r := GetRequest(ctx)
		r.AddCount("rest_calls", 1)
		r.AddDuration("rest_time", 5*time.Second)
		r.SetField("sender_id", "foobar")
		r.MeasureDuration("view_time", func() {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(200)
			w.Write([]byte(`some body`))
		})

		logger.FatalPanic(ctx, "Fifth Line")
	})

	socket, err := zmq4.NewSocket(zmq4.ROUTER)
	if err != nil {
		panic("could not create socket")
	}
	err = socket.Bind("inproc://middleware-test")
	if err != nil {
		panic("could not bind socket")
	}
	defer socket.Close()

	agentOptions := Options{
		Endpoints:    "inproc://middleware-test",
		AppName:      "appName",
		EnvName:      "envName",
		Logger:       logger,
		ObfuscateIPs: true,
	}
	agent := NewAgent(&agentOptions)
	defer agent.Shutdown()

	server := httptest.NewServer(recoveryHandler{handler: agent.NewMiddleware(router)})
	defer server.Close()

	Convey("full request/response cycle", t, func() {
		callerID := "27ce93ab-05e7-48b8-a80c-6e076c32b75a"
		actionName := "Rest::App::Vendor::V1::Users::Id#get"

		req, err := http.NewRequest("GET", server.URL+"/rest/app/vendor/v1/users/123", nil)
		req.Header.Set("X-Logjam-Caller-Id", callerID)
		req.Header.Set("Authorization", "4ec04124-bd41-49e2-9e30-5b189f5ca5f2")
		query := req.URL.Query()
		query.Set("single", "value")
		query.Set("multi", "value1")
		query.Add("multi", "value2")
		req.URL.RawQuery = query.Encode()
		So(err, ShouldBeNil)

		now := time.Now()
		res, err := server.Client().Do(req)

		So(err, ShouldBeNil)
		So(res.StatusCode, ShouldEqual, 200)
		requestID := res.Header.Get("X-Logjam-Request-Id")
		So(requestID, ShouldStartWith, "appName-envName-")
		requestParts := strings.Split(requestID, "-")
		uuid := requestParts[len(requestParts)-1]
		So(res.Header.Get("X-Logjam-Action"), ShouldEqual, actionName)
		So(res.Header.Get("X-Logjam-Caller-Id"), ShouldEqual, callerID)
		So(res.Header.Get("Http-Authorization"), ShouldEqual, "")

		msg, err := socket.RecvMessage(0)
		So(err, ShouldBeNil)
		So(msg, ShouldHaveLength, 5)

		So(msg[1], ShouldEqual, agent.AppName+"-"+agent.EnvName)
		So(msg[2], ShouldEqual, "logs."+agentOptions.AppName+"."+agentOptions.EnvName)

		payload, err := snappy.Decode(nil, []byte(msg[3]))
		So(err, ShouldBeNil)

		output := map[string]interface{}{}
		json.Unmarshal([]byte(payload), &output)

		So(output["action"], ShouldEqual, actionName)
		So(output["host"], ShouldEqual, "test-machine")
		So(output["ip"], ShouldEqual, "127.0.0.XXX")
		So(output["process_id"].(float64), ShouldNotEqual, 0)
		So(output["request_id"], ShouldEqual, uuid)
		So(output["started_at"], shouldHaveTimeFormat, timeFormat)
		startedAt, err := time.ParseInLocation(timeFormat, output["started_at"].(string), now.Location())
		So(err, ShouldBeNil)
		So(uint64(startedAt.UnixNano()/1000000), ShouldEqual, output["started_ms"])
		So(output["started_ms"], ShouldAlmostEqual, uint64(now.UnixNano()/1000000), 100)
		So(output["total_time"], ShouldBeGreaterThan, 100)
		So(output["total_time"], ShouldAlmostEqual, 100, 10)
		So(output["rest_calls"], ShouldEqual, 1)
		So(output["rest_time"], ShouldEqual, 5000)
		So(output["view_time"], ShouldBeGreaterThanOrEqualTo, 100)
		So(output["total_time"], ShouldAlmostEqual, 100, 10)
		So(output["datacenter"], ShouldEqual, "dc")
		So(output["cluster"], ShouldEqual, "a")
		So(output["namespace"], ShouldEqual, "logjam")
		So(output["sender_id"], ShouldEqual, "foobar")

		requestInfo := output["request_info"].(map[string]interface{})
		So(requestInfo["method"], ShouldEqual, "GET")

		So(requestInfo["url"], ShouldContainSubstring, "/rest/app/vendor/v1/users/123")
		So(requestInfo["url"], ShouldContainSubstring, "multi=value1")
		So(requestInfo["url"], ShouldContainSubstring, "multi=value2")
		So(requestInfo["url"], ShouldContainSubstring, "single=value")

		So(requestInfo["headers"], ShouldResemble, map[string]interface{}{
			"Accept-Encoding":    "gzip",
			"User-Agent":         "Go-http-client/1.1",
			"X-Logjam-Caller-Id": callerID,
		})

		So(requestInfo["query_parameters"], ShouldResemble, map[string]interface{}{
			"multi":  []interface{}{"value1", "value2"},
			"single": "value"})

		So(requestInfo["body_parameters"], ShouldBeNil)

		// Since Logjam requires an JSON array with mixed types, we can't express
		// it as a normal array and have to use []interface{} instead, making this
		// test a bit cumbersome.
		lines := output["lines"].([]interface{})
		So(lines, ShouldHaveLength, 6)

		line := lines[0].([]interface{})
		So(line, ShouldHaveLength, 3)
		So(line[0], ShouldEqual, DEBUG) // severity
		So(line[1], shouldHaveTimeFormat, timeFormat)
		So(line[2], ShouldEqual, "First Line")

		line = lines[1].([]interface{})
		So(line, ShouldHaveLength, 3)
		So(line[0], ShouldEqual, INFO) // severity
		So(line[1], shouldHaveTimeFormat, timeFormat)
		So(line[2], ShouldEqual, "Second Line")

		line = lines[2].([]interface{})
		So(line, ShouldHaveLength, 3)
		So(line[0], ShouldEqual, WARN) // severity
		So(line[1], shouldHaveTimeFormat, timeFormat)
		So(line[2], ShouldEqual, "Third Line")

		line = lines[3].([]interface{})
		So(line, ShouldHaveLength, 3)
		So(line[0], ShouldEqual, ERROR) // severity
		So(line[1], shouldHaveTimeFormat, timeFormat)
		So(line[2], ShouldEqual, "Fourth Line")

		line = lines[4].([]interface{})
		So(line, ShouldHaveLength, 3)
		So(line[0], ShouldEqual, FATAL) // severity
		So(line[1], shouldHaveTimeFormat, timeFormat)
		So(line[2], ShouldEqual, "Fifth Line")
	})
}

func TestSetCallHeaders(t *testing.T) {
	Convey("SetLogjamHeaders", t, func() {
		agentOptions := Options{
			Endpoints:    "127.0.0.1,localhost",
			AppName:      "appName",
			EnvName:      "envName",
			Logger:       log.New(ioutil.Discard, "", 0),
			ObfuscateIPs: true,
		}
		agent := NewAgent(&agentOptions)
		defer agent.Shutdown()

		incoming := httptest.NewRequest("GET", "/", nil)
		logjamRequest := agent.NewRequest("foobar")
		wrapped := logjamRequest.AugmentRequest(incoming)
		outgoing := httptest.NewRequest("GET", "/", nil)
		SetCallHeaders(wrapped.Context(), outgoing)
		So(outgoing.Header.Get("X-Logjam-Action"), ShouldEqual, "foobar")
		So(outgoing.Header.Get("X-Logjam-Caller-Id"), ShouldEqual, logjamRequest.id)
	})
}

func shouldHaveTimeFormat(actual interface{}, expected ...interface{}) string {
	_, err := time.Parse(expected[0].(string), actual.(string))
	if err != nil {
		return err.Error()
	}
	return ""
}
