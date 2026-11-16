package workload

import (
	"math"
	"reflect"
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
		{"mysqld", nil, "mysql"},
		{"mariadbd", nil, "mysql"},
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
		NoFileSoft: 512, NoFileHard: 4096, MemlockSoft: 8192, MaxOpenFDs: 30}
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
