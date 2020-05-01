package logjam

import (
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

	"github.com/golang/snappy"
	"github.com/gorilla/mux"
	"github.com/pebbe/zmq4"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPackInfo(t *testing.T) {
	Convey("Binary header", t, func() {
		t := time.Unix(1000000000, 1000)

		So(packInfo(t, math.MaxUint64), ShouldResemble, []byte{
			202, 189, // tag
			metaInfoCompressionMethod, // compression method
			1,                         // version
			0, 0, 0, 0,                // device
			0, 0, 0, 232, 212, 165, 16, 0, // time
			255, 255, 255, 255, 255, 255, 255, 255, // sequence
		})

		So(unpackInfo(packInfo(t, 123456789)), ShouldResemble, &metaInfo{
			Tag:               metaInfoTag,
			CompressionMethod: metaInfoCompressionMethod,
			Version:           metaInfoVersion,
			DeviceNumber:      metaInfoDeviceNumber,
			Timestamp:         uint64(t.UnixNano() / 1000000),
			Sequence:          123456789,
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

func TestLogjamHelpers(t *testing.T) {
	now := time.Date(2345, 11, 28, 23, 45, 50, 123456789, time.Now().Location())
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
	SetupAgent(&AgentOptions{Logger: log.New(fs, "API", log.LstdFlags|log.Lshortfile)})

	Convey("Log level values", t, func() {
		So(DEBUG, ShouldEqual, 0)
		So(INFO, ShouldEqual, 1)
		So(WARN, ShouldEqual, 2)
		So(ERROR, ShouldEqual, 3)
		So(FATAL, ShouldEqual, 4)
		So(UNKNOWN, ShouldEqual, 5)
	})

	now := time.Date(1970, 1, 1, 1, 0, 0, 0, time.Now().Location())

	Convey("formatLine", t, func() {
		line := formatLine(DEBUG, now, strings.Repeat("x", maxLineLength))
		So(line[0].(int), ShouldEqual, DEBUG)
		So(line[1].(string), ShouldEqual, "1970-01-01T01:00:00.000000")
		So(line[2].(string), ShouldEqual, strings.Repeat("x", maxLineLength))

		Convey("truncating message", func() {
			line := formatLine(DEBUG, now, strings.Repeat("x", 2050))
			So(line[0].(int), ShouldEqual, DEBUG)
			So(line[1].(string), ShouldEqual, "1970-01-01T01:00:00.000000")
			So(line[2].(string), ShouldEqual, strings.Repeat("x", 2027)+lineTruncated)
		})

		Convey("truncating lines", func() {
			r := Request{}
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
	setRequestEnv()

	router := mux.NewRouter()

	router.Path("/rest/e-recruiting-api/vendor/v1/users/123").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		Log(req, UNKNOWN, "First Line")
		Log(req, FATAL, "Second Line")
		Log(req, ERROR, "Third Line")
		Log(req, WARN, "Fourth Line")
		Log(req, INFO, "Sixth Line")

		r := GetRequest(req.Context())
		r.AddCount("rest_calls", 1)
		r.AddDuration("rest_time", 5*time.Second)
		r.SetField("sender_id", "foobar")
		r.MeasureDuration("view_time", func() {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(200)
			w.Write([]byte(`some body`))
		})
	})

	fs, _ := os.Open(os.DevNull)
	logger := log.New(fs, "API", log.LstdFlags|log.Lshortfile)

	agentOptions := AgentOptions{
		Endpoints: "127.0.0.1,localhost",
		AppName:   "appName",
		EnvName:   "envName",
		Logger:    logger,
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

		So(msg[1], ShouldEqual, agentOptions.AppName+"-"+agentOptions.EnvName)
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
		So(output["total_time"], ShouldAlmostEqual, 100, 5)
		So(output["datacenter"], ShouldEqual, "dc")
		So(output["cluster"], ShouldEqual, "a")
		So(output["namespace"], ShouldEqual, "logjam")
		So(output["sender_id"], ShouldEqual, "foobar")

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
		So(line[2], ShouldEqual, "First Line")

		line = lines[1].([]interface{})
		So(line, ShouldHaveLength, 3)
		So(line[0], ShouldEqual, FATAL) // severity
		So(line[1], shouldHaveTimeFormat, timeFormat)
		So(line[2], ShouldEqual, "Second Line")
	})
}

func TestSetLogjamHeaders(t *testing.T) {
	Convey("SetLogjamHeaders", t, func() {
		incoming := httptest.NewRequest("GET", "/", nil)
		logjamRequest := NewRequest("foobar")
		wrapped := logjamRequest.AugmentRequest(incoming)
		outgoing := httptest.NewRequest("GET", "/", nil)
		SetLogjamHeaders(wrapped, outgoing)
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
