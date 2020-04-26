package logjam

import (
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAgentOptionsInit(t *testing.T) {
	Convey("endpoint setup", t, func() {

		Convey("LOGJAM_AGENT_ZMQ_ENDPOINTS", func() {
			endpoints := "host1,host2"
			os.Setenv("LOGJAM_AGENT_ZMQ_ENDPOINTS", endpoints)
			SetupAgent(&AgentOptions{})
			So(agent.endpoints, ShouldResemble, []string{"tcp://host1:9604", "tcp://host2:9604"})
			// programmer values take precedence
			SetupAgent(&AgentOptions{Endpoints: "foobar", Port: 3000})
			So(agent.endpoints, ShouldResemble, []string{"tcp://foobar:3000"})
			os.Setenv("LOGJAM_AGENT_ZMQ_ENDPOINTS", "")
		})

		Convey("LOGJAM_BROKER", func() {
			endpoints := "host1"
			os.Setenv("LOGJAM_BROKER", endpoints)
			SetupAgent(&AgentOptions{})
			So(agent.endpoints, ShouldResemble, []string{"tcp://host1:9604"})
			os.Setenv("LOGJAM_BROKER", "")
		})

		Convey("empty environment", func() {
			SetupAgent(&AgentOptions{})
			So(agent.endpoints, ShouldResemble, []string{"tcp://localhost:9604"})
		})
	})
}
