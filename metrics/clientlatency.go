package metrics

import (
	"time"

	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
)

func init() {
	RegisterClientMetric("client-latency", func() interface{} {
		return &ClientLatency{}
	})
}

// ClientLatency processes LatencyMeasurementEvents, and writes LatencyMeasurements to the data logger.
type ClientLatency struct {
	mods *modules.Modules
	wf   Welford
}

// InitModule gives the module access to the other modules.
func (lr *ClientLatency) InitModule(mods *modules.Modules) {
	lr.mods = mods

	lr.mods.DataEventLoop().RegisterHandler(client.LatencyMeasurementEvent{}, func(event interface{}) {
		latencyEvent := event.(client.LatencyMeasurementEvent)
		lr.addLatency(latencyEvent.Latency)
	})

	lr.mods.DataEventLoop().RegisterObserver(types.TickEvent{}, func(event interface{}) {
		lr.tick(event.(types.TickEvent))
	})

	lr.mods.Logger().Info("Client Latency metric enabled")
}

// AddLatency adds a latency data point to the current measurement.
func (lr *ClientLatency) addLatency(latency time.Duration) {
	millis := float64(latency) / float64(time.Millisecond)
	lr.wf.Update(millis)
}

func (lr *ClientLatency) tick(tick types.TickEvent) {
	mean, variance, _ := lr.wf.Get()
	event, err := types.NewClientEvent(uint32(lr.mods.ID()), tick.Timestamp, &types.LatencyMeasurement{
		Latency:  mean,
		Variance: variance,
	})
	if err != nil {
		lr.mods.Logger().Errorf("failed to create event: %v", err)
		return
	}
	lr.mods.DataLogger().Log(event)
	lr.wf.Reset()
}
