package custompodaffinity

import (
	"testing"

	"github.com/stretchr/testify/assert"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func TestServiceUnitStats(t *testing.T) {
	app := "app"
	unit := "serviceUnit"
	spreadInfo := &extunified.PodSpreadInfo{
		AppName:     app,
		ServiceUnit: unit,
	}
	stats := newServiceUnitStats()
	stats.incCounter(app, unit, 1)
	assert.Equal(t, 1, stats.GetAllocCount(spreadInfo))
	stats.incCounter(app, unit, 1)
	stats.incCounter(app, unit, -1)
	assert.Equal(t, 1, stats.GetAllocCount(spreadInfo))
	stats.incCounter(app, unit, -1)
	stats.incCounter(app, unit, -1)
	assert.Equal(t, 0, stats.GetAllocCount(spreadInfo))

	stats.clear()
	stats2 := newServiceUnitStats()
	stats.incCounter(app, unit, 1)
	stats2.copyFrom(stats)
	assert.Equal(t, 1, stats.GetAllocCount(spreadInfo))
	assert.Equal(t, 1, stats2.GetAllocCount(spreadInfo))

	stats.add(stats2)
	assert.Equal(t, 2, stats.GetAllocCount(spreadInfo))
	assert.Equal(t, 1, stats2.GetAllocCount(spreadInfo))

	stats.setMaxValue("app2", "serviceUnit2")
	assert.Equal(t, 2, stats.GetAllocCount(spreadInfo))
	assert.Equal(t, 1, stats.GetAllocCount(&extunified.PodSpreadInfo{
		AppName:     "app2",
		ServiceUnit: "serviceUnit2",
	}))
	stats.setMaxValue(app, unit)
	assert.Equal(t, 2, stats.GetAllocCount(spreadInfo))
	assert.Equal(t, 1, stats.GetAllocCount(&extunified.PodSpreadInfo{
		AppName:     "app2",
		ServiceUnit: "serviceUnit2",
	}))

	stats.delete(stats2)
	assert.Equal(t, 1, stats.GetAllocCount(spreadInfo))
	assert.Equal(t, 1, stats.GetAllocCount(&extunified.PodSpreadInfo{
		AppName:     "app2",
		ServiceUnit: "serviceUnit2",
	}))
}
