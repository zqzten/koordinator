/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	assert.Equal(t, 1, stats.GetAllocCount(spreadInfo, nil))
	stats.incCounter(app, unit, 1)
	stats.incCounter(app, unit, -1)
	assert.Equal(t, 1, stats.GetAllocCount(spreadInfo, nil))
	stats.incCounter(app, unit, -1)
	stats.incCounter(app, unit, -1)
	assert.Equal(t, 0, stats.GetAllocCount(spreadInfo, nil))

	stats.clear()
	stats2 := newServiceUnitStats()
	stats.incCounter(app, unit, 1)
	stats2.copyFrom(stats)
	assert.Equal(t, 1, stats.GetAllocCount(spreadInfo, nil))
	assert.Equal(t, 1, stats2.GetAllocCount(spreadInfo, nil))

	stats.add(stats2)
	assert.Equal(t, 2, stats.GetAllocCount(spreadInfo, nil))
	assert.Equal(t, 1, stats2.GetAllocCount(spreadInfo, nil))

	stats.setMaxValue("app2", "serviceUnit2")
	assert.Equal(t, 2, stats.GetAllocCount(spreadInfo, nil))
	assert.Equal(t, 1, stats.GetAllocCount(&extunified.PodSpreadInfo{
		AppName:     "app2",
		ServiceUnit: "serviceUnit2",
	}, nil))
	stats.setMaxValue(app, unit)
	assert.Equal(t, 2, stats.GetAllocCount(spreadInfo, nil))
	assert.Equal(t, 1, stats.GetAllocCount(&extunified.PodSpreadInfo{
		AppName:     "app2",
		ServiceUnit: "serviceUnit2",
	}, nil))

	stats.delete(stats2)
	assert.Equal(t, 1, stats.GetAllocCount(spreadInfo, nil))
	assert.Equal(t, 1, stats.GetAllocCount(&extunified.PodSpreadInfo{
		AppName:     "app2",
		ServiceUnit: "serviceUnit2",
	}, nil))
}
