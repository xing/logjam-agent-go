package logjam

import (
	"log"
	"math"
	"os"
	"strings"
	"testing"
	"time"

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

func TestSettingFields(t *testing.T) {
	Convey("Setting fields", t, func() {
		r := NewRequest("foo")
		r.SetField("foo", "bar")
		So(r.GetField("foo"), ShouldEqual, "bar")
	})
}

func TestLog(t *testing.T) {
	fs, _ := os.Open(os.DevNull)
	SetupAgent(&AgentOptions{Logger: log.New(fs, "API", log.LstdFlags|log.Lshortfile)})

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
				r.Log(DEBUG, strings.Repeat("x", maxLineLength))
			}
			So(r.logLines, ShouldHaveLength, overflow+1)
			So(r.logLines[overflow].([]interface{})[2], ShouldEqual, linesTruncated)
		})
	})
}
