package entitlement

import (
	context "context"
	"fmt"
	"regexp"
	"sync"
	"time"

	util_log "github.com/cortexproject/cortex/pkg/util/log"
	"github.com/go-kit/log/level"
	grpc "google.golang.org/grpc"
)

// Entitlement  is a service to check entitlement
type Entitlement struct {
	entClient EntitlementClient
	entCache  sync.Map
	reLabels  sync.Map
	sync.RWMutex
	authzEnabled bool
}

// EntitlementConfig is a data structure for the Entitlement config
type EntitlementConfig struct {
	GrpcServer string `yaml:"grpc_server"`
	LabelKey   string `yaml:"label_key"`
}

type entitlementResult struct {
	timestamp int64
	entitled  bool
}

var ent *Entitlement = &Entitlement{}
var entLock *sync.RWMutex = &sync.RWMutex{}
var entConfig EntitlementConfig

func (e *Entitlement) labelValueFromLabelstring(labelKey string, labelString string) string {
	// labelString format:
	// {agent="curl", filename="/var/tmp/dummy", host="host1.example.com", job="logtest00000999"}

	var re *regexp.Regexp
	var ok bool
	var m []string
	i, ok := e.reLabels.Load(labelKey)
	if !ok {
		re = regexp.MustCompile(labelKey + `="([^"]+)"`)
		e.reLabels.Store(labelKey, re)
	} else {
		re = i.(*regexp.Regexp)
	}
	m = re.FindStringSubmatch(labelString)

	if len(m) > 0 {
		return m[1]
	}
	return ""
}

func (e *Entitlement) entConnect() {
	entLock.Lock()
	defer entLock.Unlock()

	conn, err := grpc.Dial(entConfig.GrpcServer, grpc.WithInsecure())
	if err != nil {
		level.Error(util_log.Logger).Log("msg", "grpc.Dial failed to entServer", "error", err.Error())
		return
	}

	e.entClient = NewEntitlementClient(conn)
}

// SetConfig sets entitlement config
func SetConfig(authzEnabled bool, c EntitlementConfig) {
	entLock.Lock()
	entConfig = c
	ent.authzEnabled = authzEnabled
	entLock.Unlock()
	ent.entConnect()
}

// Entitled returns true if the action/uid/labelString is entitled by the ent server
func Entitled(action string, uid string, labelString string) bool {
	// if GrpcServer is not configured, there is no entitlement check
	if ent.authzEnabled == false || entConfig.GrpcServer == "" {
		level.Debug(util_log.Logger).Log("msg", fmt.Sprintf("skipping ent check because authzEnabled:%v, GrpcServer:%v", ent.authzEnabled, entConfig.GrpcServer))
		return true
	}

	value := ent.labelValueFromLabelstring(entConfig.LabelKey, labelString)
	// 1. entitlement cache
	if entResult, ok := ent.entitledCache(action, uid, labelString); ok {
		if time.Now().Unix()-entResult.timestamp <= 60 {
			level.Debug(util_log.Logger).Log("msg",
				fmt.Sprintf("Cache found for action:%s, uid:%s, value:%s, entitled:%v, Ts:%v",
					action, uid, value, entResult.entitled, entResult.timestamp))
			return entResult.entitled
		}
		level.Debug(util_log.Logger).Log("msg",
			fmt.Sprintf("Cache expired for action:%s, uid:%s, value:%s, entitled:%v expired, Ts:%v, Now:%v. Talking to entserver",
				action, uid, value, entResult.entitled, entResult.timestamp, time.Now().Unix()))
	} else {
		level.Debug(util_log.Logger).Log("msg",
			fmt.Sprintf("Cache not found for action:%s, uid:%s, value:%s. Talking to entserver", action, uid, value))
	}

	// 2. talk to the entitlement server
	message := &EntitlementRequest{Action: action, LabelValue: value, UserID: uid}

	var res *EntitlementResponse
	var err error

	res, err = ent.entClient.Entitled(context.TODO(), message)
	if err != nil {
		ent.entConnect()
		res, err = ent.entClient.Entitled(context.TODO(), message)
	}

	if err != nil {
		level.Error(util_log.Logger).Log("msg", "failed to connect to entServer", "error", err.Error())
		return false
	}

	// cache it
	level.Debug(util_log.Logger).Log("msg", fmt.Sprintf("Cached action:%s, uid:%s, value:%s, entitled:%v", action, uid, value, res.Entitled))
	s := fmt.Sprintf("%s\t%s\t%s", action, uid, labelString)
	ent.entCache.Store(s, entitlementResult{timestamp: time.Now().Unix(), entitled: res.Entitled})

	return res.Entitled
}

// DeleteCache deletes entitlement cache
func (e *Entitlement) DeleteCache() {
	e.entCache.Range(func(key interface{}, value interface{}) bool {
		e.entCache.Delete(key)
		return true
	})
}

func (e *Entitlement) entitledCache(action string, uid string, labelString string) (entitlementResult, bool) {
	s := fmt.Sprintf("%s\t%s\t%s", action, uid, labelString)
	if item, ok := e.entCache.Load(s); ok {
		return item.(entitlementResult), true
	}
	return entitlementResult{}, false
}

func reLabelsLen() int {
	length := 0
	ent.reLabels.Range(func(_, _ interface{}) bool {
		length++
		return true
	})
	return length
}
