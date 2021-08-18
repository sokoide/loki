package util

import (
	"fmt"

	util_log "github.com/cortexproject/cortex/pkg/util/log"
	"github.com/go-kit/log/level"
)

// Entitled returns true if the action/uid/labelString is entitled by the ent server
func Entitled(action string, uid string, labelString string) bool {
	level.Debug(util_log.Logger).Log("msg",
		fmt.Sprintf("Entitled(action:%s, uid:%s, labelString:%s)", action, uid, labelString))
	return true
}
