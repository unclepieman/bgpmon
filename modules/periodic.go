package modules

import (
	"fmt"
	core "github.com/CSUNetSec/bgpmon"
	"github.com/CSUNetSec/bgpmon/util"
	"time"
)

// PeriodicModule will run another module repeatedly until it is cancelled.
type periodicModule struct {
	*BaseDaemon
}

// Run will launch the periodic daemon. Args should specify the duration,
// module to run and any arguments needed to pass to that module
// Optkeys should be: duration , module, args
// Optval args should be a proper OptString (-key val ...)
func (p *periodicModule) Run(args map[string]string, f core.FinishFunc) error {
	if !util.CheckForKeys(args, "duration", "module", "args") {
		p.logger.Errorf("Expected option keys: duration, module, args. Got %v", args)
		f()
		return nil
	}
	dval, modval, argval := args["duration"], args["module"], args["args"]
	dur, err := time.ParseDuration(dval)
	if err != nil {
		p.logger.Errorf("Error parsing duration: %s", dval)
		f()
		return nil
	}
	argmap, err := util.StringToOptMap(argval)
	if err != nil {
		p.logger.Errorf("Error %s parsing argument string: %s", err, argmap)
		f()
		return nil
	}

	tick := time.NewTicker(dur)
	defer tick.Stop()
	runC := 0
	errC := 0
	for {
		select {
		case <-p.cancel:
			p.logger.Infof("Stopping periodic")
			return nil
		case <-tick.C:
			mID := fmt.Sprintf("periodic-%s%d", modval, runC)
			err = p.server.RunModule(modval, mID, argmap)
			if err != nil {
				p.logger.Errorf("Error running module(%s): %s", modval, err)
				errC++
			} else {
				errC = 0
			}

			if errC >= 5 {
				p.logger.Errorf("Failed to run module 5 times, stopping.")
				f()
				return nil
			}
		}
		runC++
	}
}

func newPeriodicModule(s core.BgpmondServer, l util.Logger) core.Module {
	return &periodicModule{NewBaseDaemon(s, l, "periodic")}
}

func init() {
	core.RegisterModule("periodic", newPeriodicModule)
}