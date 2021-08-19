package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/loki/pkg/entitlement"
	grpc "google.golang.org/grpc"
	"gopkg.in/yaml.v2"
)

type entitlementOptions struct {
	grpcPort   int
	logLevel   string
	configFile string
}

type configRoot struct {
	Readers []string `yaml:"readers"`
	Writers []string `yaml:"writers"`
}

var options entitlementOptions = entitlementOptions{
	grpcPort:   21001,
	configFile: "entserver-sample.yml",
}

type entitlementService struct {
}

var logger log.Logger
var config configRoot
var readers map[string]bool = make(map[string]bool)
var writers map[string]bool = make(map[string]bool)

func (*entitlementService) Entitled(ctx context.Context, req *entitlement.EntitlementRequest) (*entitlement.EntitlementResponse, error) {
	var entitled bool
	switch strings.ToLower(req.Action) {
	case "write":
		if ok, value := writers[req.UserID]; ok {
			entitled = value
		} else if ok, value := writers["*"]; ok {
			entitled = value
		}
	default:
		if ok, value := readers[req.UserID]; ok {
			entitled = value
		} else if ok, value := readers["*"]; ok {
			entitled = value
		}
	}
	level.Debug(logger).Log("msg", fmt.Sprintf("UserID:%s, Action:%s, LabelValue:%s->%v", req.UserID, req.Action, req.LabelValue, entitled))

	res := &entitlement.EntitlementResponse{Entitled: entitled}
	return res, nil
}

func parseFlags() {
	flag.IntVar(&options.grpcPort, "grpcPort", options.grpcPort, "gRPC port")
	flag.StringVar(&options.configFile, "configFile", options.configFile, "config file path")
	flag.StringVar(&options.logLevel, "logLevel", options.logLevel, "NONE, DEBUG, INFO, WARNING, ERROR")
	flag.Parse()
}

func main() {
	parseFlags()
	w := log.NewSyncWriter(os.Stderr)
	logger = log.NewLogfmtLogger(w)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
	switch strings.ToUpper(options.logLevel) {
	case "ERROR":
		logger = level.NewFilter(logger, level.AllowError())
	case "WARNING":
		logger = level.NewFilter(logger, level.AllowWarn())
	case "INFO":
		logger = level.NewFilter(logger, level.AllowInfo())
	case "DEBUG":
		logger = level.NewFilter(logger, level.AllowDebug())
	case "NONE":
		logger = level.NewFilter(logger, level.AllowNone())
	default:
		logger = level.NewFilter(logger, level.AllowInfo())
		//
	}
	level.Info(logger).Log("msg", "entserver started")

	listenPort, err := net.Listen("tcp", fmt.Sprintf(":%d", options.grpcPort))
	if err != nil {
		panic(err)
	}

	data, err := ioutil.ReadFile(options.configFile)
	if err != nil {
		panic(err)
	}

	err = yaml.UnmarshalStrict(data, &config)
	if err != nil {
		panic(err)
	}

	fmt.Printf("* config: %+v\n", config)

	// convert readers/writers to map for faster query
	for _, useritem := range config.Writers {
		writers[useritem] = true
	}
	for _, user := range config.Readers {
		readers[user] = true
	}

	server := grpc.NewServer()
	service := &entitlementService{}
	entitlement.RegisterEntitlementServer(server, service)
	server.Serve(listenPort)
}
