package workload

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func root() facts.Root { return facts.Root{Proc: "testdata/proc"} }

func TestClassify(t *testing.T) {
	cases := []struct {
		comm string
		args []string
		want string
	}{
		{"python3", []string{"python3", "-m", "vllm.entrypoints.openai.api_server"}, "vllm"},
		{"vllm", []string{"/usr/local/bin/vllm", "serve", "m"}, "vllm"},
		{"python3", []string{"python3", "-m", "sglang.launch_server"}, "sglang"},
		{"text-generation", []string{"/usr/bin/text-generation-launcher"}, "tgi"},
		{"tritonserver", nil, "triton"},
		{"llama-server", nil, "llama.cpp"},
		{"llama-cli", nil, "llama.cpp"},
		{"ollama", []string{"ollama", "serve"}, "ollama"},
		{"torchrun", nil, "pytorch"},
		{"java", []string{"java", "-cp", "x", "kafka.Kafka"}, "kafka"},
		{"java", []string{"java", "org.elasticsearch.bootstrap.Elasticsearch"}, "elasticsearch"},
		{"java", []string{"java", "-jar", "app.jar"}, "jvm"},
		{"redis-server", nil, "redis"},
		{"valkey-server", nil, "valkey"},
		{"nginx", nil, "nginx"},
		{"haproxy", nil, "haproxy"},
		{"envoy", nil, "envoy"},
		{"postgres", nil, "postgres"},
		{"mysqld", []string{"/usr/sbin/mysqld", "--user=mysql"}, "mysql"},
		{"mariadbd", nil, "mariadb"},
		{"mysqld", []string{"/usr/sbin/mysqld", "--basedir=/usr/local/mariadb"}, "mariadb"},
		{"postmaster", []string{"/usr/bin/postmaster", "-D", "/var/lib/pgsql/data"}, "postgres"},
		{"java", []string{"java", "-cp", "/opt/cassandra/lib/*", "org.apache.cassandra.service.CassandraDaemon"}, "cassandra"},
		{"clickhouse-serv", []string{"/usr/bin/clickhouse-server", "--config=/etc/clickhouse-server/config.xml"}, "clickhouse"},
		{"clickhouse", []string{"clickhouse", "server", "--config-file=c.xml"}, "clickhouse"},
		{"clickhouse", []string{"clickhouse", "client"}, ""},
		{"memcached", []string{"/usr/bin/memcached", "-m", "64", "-p", "11211"}, "memcached"},
		{"java", []string{"java", "-cp", "libs/*", "kafka.Kafka", "config/server.properties"}, "kafka"},
		{"java", []string{"java", "-cp", "libs/*", "kafka.Kafka", "config/kraft/server.properties", "--override", "node.id=1"}, "kafka"},
		{"java", []string{"java", "-cp", "lib/*", "org.apache.zookeeper.server.quorum.QuorumPeerMain", "zoo.cfg"}, "zookeeper"},
		{"java", []string{"java", "-cp", "lib/*", "org.apache.pulsar.PulsarBrokerStarter"}, "pulsar"},
		{"beam.smp", []string{"/usr/lib/erlang/erts/bin/beam.smp", "-W", "w", "-A", "128", "-s", "rabbit", "boot"}, "rabbitmq"},
		{"beam.smp", []string{"beam.smp", "-W", "w", "--", "-root", "/usr/lib/erlang", "-noshell"}, ""},
		{"nats-server", []string{"nats-server", "-c", "/etc/nats.conf"}, "nats"},
		{"java", []string{"java", "org.opensearch.bootstrap.OpenSearch"}, "opensearch"},
		{"httpd", []string{"/usr/sbin/httpd", "-DFOREGROUND"}, "httpd"},
		{"apache2", []string{"/usr/sbin/apache2", "-k", "start"}, "httpd"},
		{"kubelet", []string{"/usr/bin/kubelet", "--config=/var/lib/kubelet/config.yaml"}, "kubelet"},
		{"etcd", []string{"/usr/local/bin/etcd", "--name", "n1"}, "etcd"},
		{"containerd", []string{"/usr/bin/containerd"}, "containerd"},
		{"cilium-agent", []string{"cilium-agent", "--config-dir=/tmp/cilium/config-map"}, "cilium"},
		{"coredns", []string{"/coredns", "-conf", "/etc/coredns/Corefile"}, "coredns"},
		{"prometheus", []string{"/bin/prometheus", "--config.file=/etc/prometheus/prometheus.yml"}, "prometheus"},
		{"fluent-bit", []string{"/opt/fluent-bit/bin/fluent-bit", "-c", "fb.conf"}, "fluent-bit"},
		{"td-agent-bit", []string{"/opt/td-agent-bit/bin/td-agent-bit"}, "fluent-bit"},
		{"bash", []string{"bash"}, ""},
	}
	for _, c := range cases {
		if got := classify(facts.Proc{Comm: c.comm, Cmdline: c.args}); got != c.want {
			t.Errorf("%s %v: got %q want %q", c.comm, c.args, got, c.want)
		}
	}
}

func TestDetect(t *testing.T) {
	f := &facts.Facts{Procs: []facts.Proc{
		{PID: 200, Comm: "nginx", Cmdline: []string{"nginx"}, NoFileSoft: 1024, NoFileHard: 4096, MemlockSoft: 8192, OpenFDs: 10, Unit: "nginx.service"},
		{PID: 300, Comm: "nginx", NoFileSoft: 512, NoFileHard: 8192, MemlockSoft: math.MaxUint64, OpenFDs: 30, Unit: "nginx.service"},
		{PID: 201, Comm: "redis-server", NoFileSoft: 10032, NoFileHard: 10032, MemlockSoft: 64, OpenFDs: 5},
		{PID: 400, Comm: "python3", Cmdline: []string{"python3", "train.py", "--torch"}},
		{PID: 1, Comm: "systemd"},
	}}
	w, err := Detect(root(), f)
	if err != nil {
		t.Fatal(err)
	}
	if w.Listeners != 3 || !reflect.DeepEqual(w.ListenPorts, []int{80, 7099, 8080}) {
		t.Fatalf("listeners: %d %v", w.Listeners, w.ListenPorts)
	}
	if len(w.Apps) != 2 || w.Has("pytorch") || !w.Has("nginx") || !w.Has("redis") {
		t.Fatalf("apps: %+v", w.Apps)
	}
	ng := w.Apps[0] // nginx precedes redis in table order? redis first
	for _, a := range w.Apps {
		if a.Name == "nginx" {
			ng = a
		}
	}
	want := App{Name: "nginx", PIDs: []int{200, 300}, Ports: []int{80, 8080}, Unit: "nginx.service",
		NoFileSoft: 512, NoFileHard: 4096, MemlockSoft: 8192, MaxOpenFDs: 30, NoFileSoftMax: 1024}
	if !reflect.DeepEqual(ng, want) {
		t.Fatalf("nginx: %+v", ng)
	}
	for _, a := range w.Apps {
		if a.Name == "redis" && !reflect.DeepEqual(a.Ports, []int{7099}) {
			t.Fatalf("redis ports: %v", a.Ports)
		}
	}
}

func TestDetectPytorchNeedsTorchrun(t *testing.T) {
	f := &facts.Facts{Procs: []facts.Proc{
		{PID: 1, Comm: "torchrun", Cmdline: []string{"torchrun", "x.py"}},
		{PID: 2, Comm: "python3", Cmdline: []string{"python3", "x.py", "torch"}},
	}}
	w, _ := Detect(facts.Root{Proc: "testdata/none"}, f)
	if len(w.Apps) != 1 || !reflect.DeepEqual(w.Apps[0].PIDs, []int{1, 2}) {
		t.Fatalf("%+v", w.Apps)
	}
}

func TestDetectMissingProc(t *testing.T) {
	w, err := Detect(facts.Root{Proc: "testdata/none"}, nil)
	if err != nil || w.Listeners != 0 || len(w.Apps) != 0 {
		t.Fatalf("%+v %v", w, err)
	}
}

func TestKnownApps(t *testing.T) {
	ids := KnownApps()
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate %s", id)
		}
		seen[id] = true
	}
	for _, id := range []string{"postgres", "mysql", "mariadb", "cassandra", "clickhouse", "redis", "valkey", "memcached", "kafka", "zookeeper", "pulsar", "rabbitmq", "nats", "elasticsearch", "opensearch", "nginx", "haproxy", "envoy", "httpd", "kubelet", "etcd", "containerd", "cilium", "coredns", "prometheus", "fluent-bit", "vllm", "sglang", "tgi", "triton", "llama.cpp", "ollama", "pytorch", "jvm"} {
		if !seen[id] {
			t.Errorf("missing id %s", id)
		}
	}
	if _, err := ParseDeclared([]string{"nginx", " redis ", "nginx"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDeclared([]string{"bogus"}); err == nil || !strings.Contains(err.Error(), "valid:") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeclare(t *testing.T) {
	w := &Workload{Apps: []App{{Name: "nginx", PIDs: []int{1}}}}
	w.Declare([]string{"nginx", "kafka"})
	if len(w.Apps) != 2 || w.Apps[0].Declared || !w.Apps[1].Declared || !w.Has("kafka") {
		t.Fatalf("%+v", w.Apps)
	}
}
