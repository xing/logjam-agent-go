package logjam

import (
	"encoding/json"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/facebookgo/clock"
	"github.com/gin-gonic/gin"
	"github.com/pebbe/zmq4"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	gin.SetMode(gin.TestMode)
}

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
			TimeStamp:    uint64(mockClock.Now().In(time.UTC).UnixNano() / 1000000),
			Sequence:     math.MaxUint64,
		})
	})
}

func TestActionNameFrom(t *testing.T) {
	Convey("actionNameFrom", t, func() {
		So(actionNameFrom("GET", "/swagger/index.html"), ShouldEqual, "Swagger#index.html")

		// URLs starting with _system will be ignored.
		So(actionNameFrom("GET", "_system/alive"), ShouldEqual, "_system#alive")

		v1 := "/rest/e-recruiting-api/vendor/v1/"
		So(actionNameFrom("GET", v1+"industries"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::GET#industries")
		So(actionNameFrom("GET", v1+"users/1234_foobar"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::GET::Users#by_id")
		So(actionNameFrom("GET", v1+"users"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::GET#users")
		So(actionNameFrom("GET", v1+"countries"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::GET#countries")
		So(actionNameFrom("GET", v1+"disciplines"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::GET#disciplines")
		So(actionNameFrom("GET", v1+"facets/4567"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::GET::Facets#by_id")
		So(actionNameFrom("GET", v1+"employment-statuses"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::GET#employment_statuses")
		So(actionNameFrom("DELETE", v1+"chats/123_fo"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::DELETE::Chats#by_id")
		So(actionNameFrom("GET", v1+"chats/456_bar"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::GET::Chats#by_id")
		So(actionNameFrom("GET", v1+"chats"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::GET#chats")
		So(actionNameFrom("POST", v1+"chats"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::POST#chats")
		So(actionNameFrom("PATCH", v1+"chats/123_baz"), ShouldEqual,
			"Rest::ERecruitingApi::Vendor::V1::PATCH::Chats#by_id")
	})
}

func TestLogjamHelpers(t *testing.T) {
	now := time.Date(2345, 11, 28, 23, 45, 50, 123456789, time.UTC)
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
		So(line[1].(string), ShouldEqual, "1970-01-01T00:00:00.000000")
		So(line[2].(string), ShouldEqual, strings.Repeat("x", maxLineLength))

		Convey("truncating message", func() {
			line := formatLine(DEBUG, mockClock.Now(), strings.Repeat("x", 2050))
			So(line[0].(int), ShouldEqual, DEBUG)
			So(line[1].(string), ShouldEqual, "1970-01-01T00:00:00.000000")
			So(line[2].(string), ShouldEqual, strings.Repeat("x", 2027)+lineTruncated)
		})

		Convey("truncating lines", func() {
			r := request{middleware: &middleware{Options: &Options{Clock: mockClock}}}
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
	Convey("the channel", t, func() {
		router := gin.New()
		options := &Options{Endpoints: ""}

		Convey("LOGJAM_AGENT_ZMQ_ENDPOINTS", func() {
			endpoints := "broker-1.monitor.ams1.xing.com,broker-2.monitor.ams1.xing.com,broker-3.monitor.ams1.xing.com"
			os.Setenv("LOGJAM_AGENT_ZMQ_ENDPOINTS", endpoints)
			mw := NewMiddleware(router, options).(*middleware)
			os.Setenv("LOGJAM_AGENT_ZMQ_ENDPOINTS", "")
			So(mw.Endpoints, ShouldEqual, endpoints)
		})

		Convey("LOGJAM_BROKER", func() {
			endpoints := "broker.monitor.ams1.xing.com"
			os.Setenv("LOGJAM_BROKER", endpoints)
			mw := NewMiddleware(router, options).(*middleware)
			os.Setenv("LOGJAM_BROKER", "")
			So(mw.Endpoints, ShouldEqual, endpoints)
		})

		Convey("default value", func() {
			mw := NewMiddleware(router, options).(*middleware)
			So(mw.Endpoints, ShouldEqual, "localhost")
		})
	})
}

func TestMiddleware(t *testing.T) {
	os.Setenv("HOSTNAME", "test-machine")
	mockClock := clock.NewMock()
	now := time.Duration(1519659204000000000)
	mockClock.Add(now)

	router := gin.New()

	router.GET("/rest/e-recruiting-api/vendor/v1/users/123", func(c *gin.Context) {
		mockClock.Add(1 * time.Second)
		Log(c.Request, UNKNOWN, "First Line")
		mockClock.Add(1 * time.Second)
		Log(c.Request, FATAL, "Second Line")
		mockClock.Add(1 * time.Second)
		Log(c.Request, ERROR, "Third Line")
		mockClock.Add(1 * time.Second)
		Log(c.Request, WARN, "Fourth Line")
		mockClock.Add(1 * time.Second)
		Log(c.Request, INFO, "Sixth Line")

		AddCount(c.Request, "RestCalls", 1)
		AddDuration(c.Request, "RestTime", 5*time.Second)
		AddDurationFunc(c.Request, "ViewTime", func() {
			mockClock.Add(5 * time.Second)
			c.String(200, "some body")
		})
	})

	options := &Options{
		Endpoints:    "127.0.0.1,localhost",
		AppName:      "appName",
		EnvName:      "envName",
		Clock:        mockClock,
		RandomSource: rand.New(rand.NewSource(123)),
	}

	server := httptest.NewServer(NewMiddleware(router, options))
	defer server.Close()

	Convey("full request/response cycle", t, func() {
		socket, err := zmq4.NewSocket(zmq4.ROUTER)
		So(err, ShouldBeNil)
		socket.Bind("tcp://*:9604")
		defer func() {
			socket.Unbind("tcp://*:9604")
		}()

		uuid := "405ced8b99684af9909259515bf7025a"
		callerId := "27ce93ab-05e7-48b8-a80c-6e076c32b75a"
		actionName := "Rest::ERecruitingApi::Vendor::V1::GET::Users#by_id"

		req, err := http.NewRequest("GET", server.URL+"/rest/e-recruiting-api/vendor/v1/users/123", nil)
		req.Header.Set("X-Logjam-Caller-Id", callerId)
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
		So(res.Header.Get("X-Logjam-Request-Id"), ShouldEqual, "appName-envName-"+uuid)
		So(res.Header.Get("X-Logjam-Request-Action"), ShouldEqual, actionName)
		So(res.Header.Get("X-Logjam-Caller-Id"), ShouldEqual, callerId)
		So(res.Header.Get("Http-Authorization"), ShouldEqual, "")

		msg, err := socket.RecvMessage(0)
		So(err, ShouldBeNil)
		So(msg, ShouldHaveLength, 5)

		So(msg[1], ShouldEqual, options.AppName+"-"+options.EnvName)
		So(msg[2], ShouldEqual, "logs."+options.AppName+"."+options.EnvName)

		output := map[string]interface{}{}
		json.Unmarshal([]byte(msg[3]), &output)

		So(output["action"], ShouldEqual, actionName)
		So(output["host"], ShouldEqual, "test-machine")
		So(output["ip"], ShouldEqual, "127.0.0.XXX")
		So(output["process_id"].(float64), ShouldNotEqual, 0)
		So(output["request_id"], ShouldEqual, uuid)
		So(output["started_at"], shouldHaveTimeFormat, iso8601)
		So(output["started_at"], ShouldEqual, "2018-02-26T15:33:24+07:00")
		So(output["started_ms"], ShouldAlmostEqual, float64(now)/1000000) // this test will fail after 2286-11-20
		So(output["total_time"], ShouldEqual, 10000)
		So(output["rest_calls"], ShouldEqual, 1)
		So(output["rest_time"], ShouldEqual, 5000)
		So(output["view_time"], ShouldEqual, 5000)

		requestInfo := output["request_info"].(map[string]interface{})
		So(requestInfo["method"], ShouldEqual, "GET")

		So(requestInfo["url"], ShouldContainSubstring, "/rest/e-recruiting-api/vendor/v1/users/123")
		So(requestInfo["url"], ShouldContainSubstring, "multi=value1")
		So(requestInfo["url"], ShouldContainSubstring, "multi=value2")
		So(requestInfo["url"], ShouldContainSubstring, "single=value")

		So(requestInfo["headers"], ShouldResemble, map[string]interface{}{
			"Accept-Encoding":    "gzip",
			"User-Agent":         "Go-http-client/1.1",
			"X-Logjam-Caller-Id": callerId,
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
		So(line[1], ShouldEqual, "2018-02-26T15:33:25.000000")
		So(line[2], ShouldEqual, "First Line")

		line = lines[1].([]interface{})
		So(line, ShouldHaveLength, 3)
		So(line[0], ShouldEqual, FATAL) // severity
		So(line[1], ShouldEqual, "2018-02-26T15:33:26.000000")
		So(line[2], ShouldEqual, "Second Line")
	})
}

func shouldHaveTimeFormat(actual interface{}, expected ...interface{}) string {
	_, err := time.Parse(expected[0].(string), actual.(string))
	if err != nil {
		return err.Error()
	}
	return ""
}
