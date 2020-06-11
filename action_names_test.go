package logjam

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLegacyActionNameExtractor(t *testing.T) {
	Convey("LegacyActionNameExtractor", t, func() {
		Convey("extracting action names", func() {
			So(legacyActionNameFrom("GET", "/"), ShouldEqual,
				"Unknown#unknown")
			So(legacyActionNameFrom("GET", "/something"), ShouldEqual,
				"Unknown#something")

			So(legacyActionNameFrom("GET", "/swagger/index.html"), ShouldEqual,
				"Swagger#index.html")

			// URLs starting with _system will be ignored.
			So(legacyActionNameFrom("GET", "/_system/alive"), ShouldEqual,
				"_system#alive")

			v1 := "/rest/app/vendor/v1/"
			So(legacyActionNameFrom("GET", v1+"industries"), ShouldEqual,
				"Rest::App::Vendor::V1::GET#industries")
			So(legacyActionNameFrom("GET", v1+"users/1234_foobar"), ShouldEqual,
				"Rest::App::Vendor::V1::GET::Users#by_id")
			So(legacyActionNameFrom("GET", v1+"users"), ShouldEqual,
				"Rest::App::Vendor::V1::GET#users")
			So(legacyActionNameFrom("GET", v1+"countries"), ShouldEqual,
				"Rest::App::Vendor::V1::GET#countries")
			So(legacyActionNameFrom("GET", v1+"disciplines"), ShouldEqual,
				"Rest::App::Vendor::V1::GET#disciplines")
			So(legacyActionNameFrom("GET", v1+"facets/4567"), ShouldEqual,
				"Rest::App::Vendor::V1::GET::Facets#by_id")
			So(legacyActionNameFrom("GET", v1+"employment-statuses"), ShouldEqual,
				"Rest::App::Vendor::V1::GET#employment_statuses")
			So(legacyActionNameFrom("DELETE", v1+"chats/123_fo"), ShouldEqual,
				"Rest::App::Vendor::V1::DELETE::Chats#by_id")
			So(legacyActionNameFrom("GET", v1+"chats/456_bar"), ShouldEqual,
				"Rest::App::Vendor::V1::GET::Chats#by_id")
			So(legacyActionNameFrom("GET", v1+"chats"), ShouldEqual,
				"Rest::App::Vendor::V1::GET#chats")
			So(legacyActionNameFrom("POST", v1+"chats"), ShouldEqual,
				"Rest::App::Vendor::V1::POST#chats")
			So(legacyActionNameFrom("PATCH", v1+"chats/123_baz"), ShouldEqual,
				"Rest::App::Vendor::V1::PATCH::Chats#by_id")
		})
	})
}

func TestDefaultActionNameExtractor(t *testing.T) {
	Convey("DefaultActionNameExtractor", t, func() {
		Convey("extracting action names", func() {
			So(defaultActionNameFrom("GET", "/"), ShouldEqual,
				"Unknown#get")
			So(defaultActionNameFrom("GET", "/something"), ShouldEqual,
				"Something#get")

			So(defaultActionNameFrom("GET", "/swagger/index.html"), ShouldEqual,
				"Swagger::Index.Html#get")

			// URLs starting with _system will be ignored.
			So(defaultActionNameFrom("GET", "/_system/alive"), ShouldEqual,
				"System::Alive#get")

			v1 := "/rest/app/vendor/v1/"
			So(defaultActionNameFrom("GET", v1+"industries"), ShouldEqual,
				"Rest::App::Vendor::V1::Industries#get")
			So(defaultActionNameFrom("GET", v1+"users/1234_foobar"), ShouldEqual,
				"Rest::App::Vendor::V1::Users::Id#get")
			So(defaultActionNameFrom("GET", v1+"users"), ShouldEqual,
				"Rest::App::Vendor::V1::Users#get")
			So(defaultActionNameFrom("GET", v1+"countries"), ShouldEqual,
				"Rest::App::Vendor::V1::Countries#get")
			So(defaultActionNameFrom("GET", v1+"disciplines"), ShouldEqual,
				"Rest::App::Vendor::V1::Disciplines#get")
			So(defaultActionNameFrom("GET", v1+"facets/4567"), ShouldEqual,
				"Rest::App::Vendor::V1::Facets::Id#get")
			So(defaultActionNameFrom("GET", v1+"employment-statuses"), ShouldEqual,
				"Rest::App::Vendor::V1::EmploymentStatuses#get")
			So(defaultActionNameFrom("DELETE", v1+"chats/123_fo"), ShouldEqual,
				"Rest::App::Vendor::V1::Chats::Id#delete")
			So(defaultActionNameFrom("GET", v1+"chats/456_bar"), ShouldEqual,
				"Rest::App::Vendor::V1::Chats::Id#get")
			So(defaultActionNameFrom("GET", v1+"chats"), ShouldEqual,
				"Rest::App::Vendor::V1::Chats#get")
			So(defaultActionNameFrom("POST", v1+"chats"), ShouldEqual,
				"Rest::App::Vendor::V1::Chats#post")
			So(defaultActionNameFrom("PATCH", v1+"chats/123_baz"), ShouldEqual,
				"Rest::App::Vendor::V1::Chats::Id#patch")
		})
	})
}
