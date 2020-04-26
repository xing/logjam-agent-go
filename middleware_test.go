package logjam

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"log"

	"github.com/facebookgo/clock"
	"github.com/gorilla/mux"
	"github.com/pebbe/zmq4"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPackInfo(t *testing.T) {
	Convey("Binary header", t, func() {
		mockClock := clock.NewMock()
		mockClock.Add(time.Duration(1519659204000000000))

		So(packInfo(mockClock, math.MaxUint64), ShouldResemble, []byte{
			202, 189, // tag
			0,          // compression method
			1,          // version
			0, 0, 0, 0, // device
			0, 0, 1, 97, 210, 191, 61, 160, // time
			255, 255, 255, 255, 255, 255, 255, 255, // sequence
		})

		So(unpackInfo(packInfo(mockClock, math.MaxUint64)), ShouldResemble, &metaInfo{
			Tag:          metaInfoTag,
			Version:      metaInfoVersion,
			DeviceNumber: metaInfoDeviceNumber,
			TimeStamp:    uint64(mockClock.Now().In(timeLocation).UnixNano() / 1000000),
			Sequence:     math.MaxUint64,
		})
	})
}

func extractActionName(mw *middleware, method string, path string) string {
	return mw.ActionNameExtractor(httptest.NewRequest(method, path, nil))
}

func TestActionNameExtractor(t *testing.T) {
	Convey("ActionNameExtractor", t, func() {
		router := mux.NewRouter()
		Convey("when set uses it", func() {
			options := &MiddlewareOptions{
				ActionNameExtractor: func(r *http.Request) string {
					return fmt.Sprintf("%s_userdefined", r.Method)
				},
			}
			mw := NewMiddleware(router, options).(*middleware)

			So(extractActionName(mw, "GET", "/some/path"), ShouldEqual, "GET_userdefined")
		})

		Convey("when unset uses default", func() {
			options := &MiddlewareOptions{}
			mw := NewMiddleware(router, options).(*middleware)

			So(extractActionName(mw, "GET", "/swagger/index.html"), ShouldEqual,
				"Swagger#index.html")

			// URLs starting with _system will be ignored.
			So(extractActionName(mw, "GET", "/_system/alive"), ShouldEqual,
				"_system#alive")

			v1 := "/rest/e-recruiting-api/vendor/v1/"
			So(extractActionName(mw, "GET", v1+"industries"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::GET#industries")
			So(extractActionName(mw, "GET", v1+"users/1234_foobar"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::GET::Users#by_id")
			So(extractActionName(mw, "GET", v1+"users"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::GET#users")
			So(extractActionName(mw, "GET", v1+"countries"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::GET#countries")
			So(extractActionName(mw, "GET", v1+"disciplines"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::GET#disciplines")
			So(extractActionName(mw, "GET", v1+"facets/4567"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::GET::Facets#by_id")
			So(extractActionName(mw, "GET", v1+"employment-statuses"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::GET#employment_statuses")
			So(extractActionName(mw, "DELETE", v1+"chats/123_fo"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::DELETE::Chats#by_id")
			So(extractActionName(mw, "GET", v1+"chats/456_bar"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::GET::Chats#by_id")
			So(extractActionName(mw, "GET", v1+"chats"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::GET#chats")
			So(extractActionName(mw, "POST", v1+"chats"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::POST#chats")
			So(extractActionName(mw, "PATCH", v1+"chats/123_baz"), ShouldEqual,
				"Rest::ERecruitingApi::Vendor::V1::PATCH::Chats#by_id")
		})
	})
}

func TestNewRequest(t *testing.T) {
	Convey("actionName", t, func() {
		Convey("uses default extractor", func() {
			router := mux.NewRouter()
			options := &MiddlewareOptions{}
			mw := NewMiddleware(router, options).(*middleware)
			logjamRequest := mw.newRequest(httptest.NewRequest("GET", "/some/action", nil), httptest.NewRecorder())

			So(logjamRequest.actionName, ShouldEqual, "Some#action")
		})

		Convey("uses configured extractor", func() {
			router := mux.NewRouter()
			options := &MiddlewareOptions{
				ActionNameExtractor: func(r *http.Request) string {
					return fmt.Sprintf("%s::such::generated", r.Method)
				},
			}
			mw := NewMiddleware(router, options).(*middleware)
			logjamRequest := mw.newRequest(httptest.NewRequest("GET", "/some/action", nil), httptest.NewRecorder())

			So(logjamRequest.actionName, ShouldEqual, "GET::such::generated")
		})
	})
}

func TestLogjamHelpers(t *testing.T) {
	now := time.Date(2345, 11, 28, 23, 45, 50, 123456789, timeLocation)
	nowString := "2345-11-28T23:45:50.123456"

	Convey("Logjam helpers", t, func() {
		Convey("Formats time in logjam format", func() {
			So(formatTime(now), ShouldEqual, nowString)
		})

		Convey("Creates a logjam compatible log line", func() {
			line := formatLine(1, now, "Some text")
			So(line[0], ShouldEqual, 1)
			So(line[1], ShouldEqual, nowString)
			So(line[2], ShouldEqual, "Some text")
		})
	})
}

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

func TestLog(t *testing.T) {
	fs, _ := os.Open(os.DevNull)
	logger = log.New(fs, "API", log.LstdFlags|log.Lshortfile)

	Convey("Log level values", t, func() {
		So(DEBUG, ShouldEqual, 0)
		So(INFO, ShouldEqual, 1)
		So(WARN, ShouldEqual, 2)
		So(ERROR, ShouldEqual, 3)
		So(FATAL, ShouldEqual, 4)
		So(UNKNOWN, ShouldEqual, 5)
	})

	mockClock := clock.NewMock()

	Convey("formatLine", t, func() {
		line := formatLine(DEBUG, mockClock.Now(), strings.Repeat("x", maxLineLength))
		So(line[0].(int), ShouldEqual, DEBUG)
		So(line[1].(string), ShouldEqual, "1970-01-01T01:00:00.000000")
		So(line[2].(string), ShouldEqual, strings.Repeat("x", maxLineLength))

		Convey("truncating message", func() {
			line := formatLine(DEBUG, mockClock.Now(), strings.Repeat("x", 2050))
			So(line[0].(int), ShouldEqual, DEBUG)
			So(line[1].(string), ShouldEqual, "1970-01-01T01:00:00.000000")
			So(line[2].(string), ShouldEqual, strings.Repeat("x", 2027)+lineTruncated)
		})

		Convey("truncating lines", func() {
			r := request{}
			overflow := (maxBytesAllLines / maxLineLength)
			for i := 0; i < overflow*2; i++ {
				r.log(DEBUG, strings.Repeat("x", maxLineLength))
			}
			So(r.logLines, ShouldHaveLength, overflow+1)
			So(r.logLines[overflow].([]interface{})[2], ShouldEqual, linesTruncated)
		})
	})
}

func TestMiddlewareOptionsInit(t *testing.T) {
	Convey("new middleware", t, func() {
	})
}

func TestMiddleware(t *testing.T) {
	os.Setenv("HOSTNAME", "test-machine")
	os.Setenv("DATACENTER", "dc")
	os.Setenv("CLUSTER", "a")
	os.Setenv("NAMESPACE", "logjam")

	mockClock := clock.NewMock()
	now := time.Duration(1519659204000000000)
	mockClock.Add(now)

	router := mux.NewRouter()

	router.Path("/rest/e-recruiting-api/vendor/v1/users/123").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mockClock.Add(1 * time.Second)
		Log(req, UNKNOWN, "First Line")
		mockClock.Add(1 * time.Second)
		Log(req, FATAL, "Second Line")
		mockClock.Add(1 * time.Second)
		Log(req, ERROR, "Third Line")
		mockClock.Add(1 * time.Second)
		Log(req, WARN, "Fourth Line")
		mockClock.Add(1 * time.Second)
		Log(req, INFO, "Sixth Line")

		AddCount(req, "RestCalls", 1)
		AddDuration(req, "RestTime", 5*time.Second)
		AddDurationFunc(req, "ViewTime", func() {
			mockClock.Add(5 * time.Second)
			w.WriteHeader(200)
			w.Write([]byte(`some body`))
		})
	})

	agentOptions := AgentOptions{
		Endpoints: "127.0.0.1,localhost",
		AppName:   "appName",
		EnvName:   "envName",
		Clock:     mockClock,
	}
	SetupAgent(&agentOptions)
	defer ShutdownAgent()

	server := httptest.NewServer(NewMiddleware(router, &MiddlewareOptions{}))
	defer server.Close()

	Convey("full request/response cycle", t, func() {
		socket, err := zmq4.NewSocket(zmq4.ROUTER)
		So(err, ShouldBeNil)
		socket.Bind("tcp://*:9604")
		defer func() {
			socket.Unbind("tcp://*:9604")
		}()

		callerID := "27ce93ab-05e7-48b8-a80c-6e076c32b75a"
		actionName := "Rest::ERecruitingApi::Vendor::V1::GET::Users#by_id"

		req, err := http.NewRequest("GET", server.URL+"/rest/e-recruiting-api/vendor/v1/users/123", nil)
		req.Header.Set("X-Logjam-Caller-Id", callerID)
		req.Header.Set("Authorization", "4ec04124-bd41-49e2-9e30-5b189f5ca5f2")
		query := req.URL.Query()
		query.Set("single", "value")
		query.Set("multi", "value1")
		query.Add("multi", "value2")
		req.URL.RawQuery = query.Encode()
		So(err, ShouldBeNil)
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

		So(msg[1], ShouldEqual, agentOptions.AppName+"-"+agentOptions.EnvName)
		So(msg[2], ShouldEqual, "logs."+agentOptions.AppName+"."+agentOptions.EnvName)

		output := map[string]interface{}{}
		json.Unmarshal([]byte(msg[3]), &output)

		So(output["action"], ShouldEqual, actionName)
		So(output["host"], ShouldEqual, "test-machine")
		So(output["ip"], ShouldEqual, "127.0.0.XXX")
		So(output["process_id"].(float64), ShouldNotEqual, 0)
		So(output["request_id"], ShouldEqual, uuid)
		So(output["started_at"], shouldHaveTimeFormat, time.RFC3339)
		So(output["started_at"], ShouldEqual, "2018-02-26T16:33:24+01:00")
		So(output["started_ms"], ShouldAlmostEqual, float64(now)/1000000) // this test will fail after 2286-11-20
		So(output["total_time"], ShouldEqual, 10000)
		So(output["rest_calls"], ShouldEqual, 1)
		So(output["rest_time"], ShouldEqual, 5000)
		So(output["view_time"], ShouldEqual, 5000)
		So(output["datacenter"], ShouldEqual, "dc")
		So(output["cluster"], ShouldEqual, "a")
		So(output["namespace"], ShouldEqual, "logjam")

		requestInfo := output["request_info"].(map[string]interface{})
		So(requestInfo["method"], ShouldEqual, "GET")

		So(requestInfo["url"], ShouldContainSubstring, "/rest/e-recruiting-api/vendor/v1/users/123")
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
		So(lines, ShouldHaveLength, 5)

		line := lines[0].([]interface{})
		So(line, ShouldHaveLength, 3)
		So(line[0], ShouldEqual, UNKNOWN) // severity
		So(line[1], shouldHaveTimeFormat, timeFormat)
		So(line[1], ShouldEqual, "2018-02-26T16:33:25.000000")
		So(line[2], ShouldEqual, "First Line")

		line = lines[1].([]interface{})
		So(line, ShouldHaveLength, 3)
		So(line[0], ShouldEqual, FATAL) // severity
		So(line[1], ShouldEqual, "2018-02-26T16:33:26.000000")
		So(line[2], ShouldEqual, "Second Line")
	})
}

func TestSetLogjamHeaders(t *testing.T) {
	Convey("SetLogjamHeaders", t, func() {
		router := mux.NewRouter()
		mw := NewMiddleware(router, &MiddlewareOptions{
			ActionNameExtractor: func(r *http.Request) string {
				return fmt.Sprintf("%s_userdefined", r.Method)
			},
		}).(*middleware)
		incoming := httptest.NewRequest("GET", "/", nil)
		res := httptest.NewRecorder()
		wrapped := mw.newRequest(incoming, res)
		wrapped.start()
		incomingW := incoming.WithContext(context.WithValue(incoming.Context(), requestKey, wrapped))
		outgoing := httptest.NewRequest("GET", "/", nil)
		SetLogjamHeaders(incomingW, outgoing)
		So(outgoing.Header.Get("X-Logjam-Action"), ShouldEqual, "GET_userdefined")
		So(outgoing.Header.Get("X-Logjam-Caller-Id"), ShouldEqual, wrapped.id)
	})
}

func shouldHaveTimeFormat(actual interface{}, expected ...interface{}) string {
	_, err := time.Parse(expected[0].(string), actual.(string))
	if err != nil {
		return err.Error()
	}
	return ""
}
